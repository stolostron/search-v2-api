// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/driftprogramming/pgxpoolmock"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
	db "github.com/stolostron/search-v2-api/pkg/database"
	"k8s.io/klog/v2"
)

type SearchResult struct {
	input  *model.SearchInput
	pool   pgxpoolmock.PgxPool
	uids   []string       // List of uids from search result to be used to get relatioinships.
	wg     sync.WaitGroup // WORKAROUND: Used to serialize search query and relatioinships query.
	query  string
	params []interface{}
	// 	Count   int
	// 	Items   []map[string]interface{}
	//  Related []SearchRelatedResult
}

func Search(ctx context.Context, input []*model.SearchInput) ([]*SearchResult, error) {
	// For each input, create a SearchResult resolver.
	srchResult := make([]*SearchResult, len(input))
	if len(input) > 0 {
		for index, in := range input {
			srchResult[index] = &SearchResult{
				input: in,
				pool:  db.GetConnection(),
			}
		}
	}
	return srchResult, nil
}

func (s *SearchResult) Count() int {
	klog.V(2).Info("Resolving SearchResult:Count()")
	s.buildSearchQuery(context.Background(), true)
	count, e := s.resolveCount()

	if e != nil {
		klog.Error("Error resolving count.", e)
	}
	return count
}

func (s *SearchResult) Items() []map[string]interface{} {
	s.wg.Add(1)
	defer s.wg.Done()
	klog.V(2).Info("Resolving SearchResult:Items()")
	s.buildSearchQuery(context.Background(), false)
	r, e := s.resolveItems()
	if e != nil {
		klog.Error("Error resolving items.", e)
	}
	return r
}

func (s *SearchResult) Related() []SearchRelatedResult {
	klog.V(2).Info("Resolving SearchResult:Related()")

	// FIXME: WORKAROUND when the query doesn't request Items() we must use a more efficient query to get the uids.
	if s.uids == nil {
		klog.Warning("TODO: Use a query that only selects the UIDs.")
		s.Items()
	}
	s.wg.Wait() // WORKAROUND wait to complete execution of Items()

	r := s.getRelations()
	return r
}

func (s *SearchResult) buildSearchQuery(ctx context.Context, count bool) {
	var limit int
	var selectDs *goqu.SelectDataset
	var whereDs []exp.Expression

	// Example query: SELECT uid, cluster, data FROM search.resources  WHERE lower(data->> 'kind') IN
	// (lower('Pod')) AND lower(data->> 'cluster') IN (lower('local-cluster')) LIMIT 10000
	//FROM CLAUSE
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)
	//SELECT CLAUSE
	if count {
		selectDs = ds.Select(goqu.COUNT("uid"))
	} else {
		selectDs = ds.Select("uid", "cluster", "data")
	}
	//WHERE CLAUSE
	if s.input != nil && len(s.input.Filters) > 0 {
		whereDs = WhereClauseFilter(s.input)
	}
	//LIMIT CLAUSE
	if s.input != nil && s.input.Limit != nil && *s.input.Limit != 0 {
		limit = *s.input.Limit
	} else {
		limit = config.DEFAULT_QUERY_LIMIT
	}
	//Get the query
	sql, params, err := selectDs.Where(whereDs...).Limit(uint(limit)).ToSQL()
	if err != nil {
		klog.Errorf("Error building SearchComplete query: %s", err.Error())
	}
	klog.Infof("query: %s\nargs: %s", sql, params)
	s.query = sql
	s.params = params
}

func (s *SearchResult) resolveCount() (int, error) {
	rows := s.pool.QueryRow(context.Background(), s.query, s.params...)

	var count int
	err := rows.Scan(&count)

	return count, err
}

func (s *SearchResult) resolveItems() ([]map[string]interface{}, error) {
	rows, err := s.pool.Query(context.Background(), s.query, s.params...)
	if err != nil {
		klog.Errorf("Error resolving query [%s] with args [%+v]. Error: [%+v]", s.query, s.params, err)
	}
	defer rows.Close()

	var uid, cluster string
	var data map[string]interface{}
	items := []map[string]interface{}{}
	s.uids = make([]string, len(items))

	for rows.Next() {
		err = rows.Scan(&uid, &cluster, &data)
		if err != nil {
			klog.Errorf("Error %s retrieving rows for query:%s", err.Error(), s.query)
		}

		currItem := formatDataMap(data)
		currItem["_uid"] = uid
		currItem["cluster"] = cluster

		items = append(items, currItem)
		s.uids = append(s.uids, uid)
	}

	return items, nil
}

func (s *SearchResult) getRelations() []SearchRelatedResult {
	klog.Infof("Resolving relationships for [%d] uids.\n", len(s.uids))

	if len(s.input.RelatedKinds) > 0 {
		// TODO: Use the RelatedKinds filter in the SQL query.
		klog.Warning("TODO: The relationships query must use the provided kind filters.")
	}

	//defining variables
	items := []map[string]interface{}{}
	var kindSlice []string
	var kindList []string
	var countList []int

	// LEARNING: IN is equivalent to = ANY and performance is not deteriorated when we replace IN with =ANY
	recrusiveQuery := `with recursive
	search_graph(uid, data, sourcekind, destkind, sourceid, destid, path, level)
	as (
	SELECT r.uid, r.data, e.sourcekind, e.destkind, e.sourceid, e.destid, ARRAY[r.uid] as path, 1 as level
		from search.resources r
		INNER JOIN
			search.edges e ON (r.uid = e.sourceid) or (r.uid = e.destid)
		 where r.uid = ANY($1)
	union
	select r.uid, r.data, e.sourcekind, e.destkind, e.sourceid, e.destid, path||r.uid, level+1 as level
		from search.resources r
		INNER JOIN
			search.edges e ON (r.uid = e.sourceid)
		, search_graph sg
		where (e.sourceid = sg.destid or e.destid = sg.sourceid)
		and r.uid <> all(sg.path)
		and level = 1 
		)
	select distinct on (destid) data, destid, destkind from search_graph where level=1 or destid = ANY($2)`

	relations, QueryError := s.pool.Query(context.Background(), recrusiveQuery, s.uids, s.uids) // how to deal with defaults.
	if QueryError != nil {
		klog.Errorf("query error :", QueryError)
	}

	defer relations.Close()

	// iterating through resulting rows and scaning data, destid  and destkind
	for relations.Next() {
		var destkind, destid string
		var data map[string]interface{}
		relatedResultError := relations.Scan(&data, &destid, &destkind)
		if relatedResultError != nil {
			klog.Errorf("Error %s retrieving rows for relationships:%s", relatedResultError.Error(), relations)
		}

		// creating currItem variable to keep data and converting strings in data to lowercase
		currItem := formatDataMap(data)

		// currItem["Kind"] = destkind
		kindSlice = append(kindSlice, destkind)
		items = append(items, currItem)

	}

	//calling function to get map which contains unique values from kindSlice and counts the number occurances ex: map[key:Pod, value:2] if pod occurs 2x in kindSlice
	count := printUniqueValue(kindSlice)

	//iterating over count and appending to new lists (kindList and countList)
	for k, v := range count {
		// fmt.Println("Keys:", k)
		kindList = append(kindList, k)
		// fmt.Println("Values:", v)
		countList = append(countList, v)
	}

	//instantiating composite literal
	relatedSearch := make([]SearchRelatedResult, len(count))

	//iterating and sending values to relatedSearch
	for i := range kindList {
		kind := kindList[i]
		count := countList[i]
		relatedSearch[i] = SearchRelatedResult{kind, &count, items}
	}

	return relatedSearch
}

// helper function TODO: make helper.go module to store these if needed.
func printUniqueValue(arr []string) map[string]int {
	// Create a dictionary of values for each element
	dict := make(map[string]int)
	for _, num := range arr {
		dict[num] = dict[num] + 1
	}
	return dict
}

// Labels are sorted alphabetically to ensure consistency, then encoded in a
// string with the following format.
// key1:value1,key2:value2,...
func formatLabels(labels map[string]interface{}) string {
	keys := make([]string, 0)
	labelStrings := make([]string, 0)
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		labelStrings = append(labelStrings, fmt.Sprintf("%s:%s", k, labels[k]))
	}
	return strings.Join(labelStrings, ",")
}

// Encode array into a single string with the format.
//  value1,value2,...
func formatArray(itemlist []interface{}) string {
	keys := make([]string, len(itemlist))
	for i, k := range itemlist {
		keys[i] = formatItem(k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

// Convert interface to string format
func formatItem(data interface{}) string {
	var item string
	switch v := data.(type) {
	case string:
		item = strings.ToLower(v)
	case bool:
		item = strconv.FormatBool(v)
	case float64:
		item = strconv.FormatInt(int64(v), 10)
	default:
		klog.Warningf("Error formatting property with type: %+v\n", reflect.TypeOf(v))
	}
	return item
}

func formatDataMap(data map[string]interface{}) map[string]interface{} {
	item := make(map[string]interface{})
	for key, value := range data {
		switch v := value.(type) {
		case string:
			item[key] = strings.ToLower(v)
		case bool:
			item[key] = strconv.FormatBool(v)
		case float64:
			item[key] = strconv.FormatInt(int64(v), 10)
		case map[string]interface{}:
			item[key] = formatLabels(v)
		case []interface{}:
			item[key] = formatArray(v)
		default:
			klog.Warningf("Error formatting property with key: %+v  type: %+v\n", key, reflect.TypeOf(v))
			continue
		}
	}
	return item
}

func WhereClauseFilter(input *model.SearchInput) []exp.Expression {
	var whereDs []exp.Expression
	for _, filter := range input.Filters {
		var values []string
		if len(filter.Values) > 0 {
			for _, val := range filter.Values {
				values = append(values, *val)
			}
		}
		if filter.Property == "cluster" {
			whereDs = append(whereDs, goqu.C(filter.Property).In(values).Expression())
		} else {
			whereDs = append(whereDs, goqu.L(`"data"->>?`, filter.Property).In(values).Expression())
		}
	}
	return whereDs
}
