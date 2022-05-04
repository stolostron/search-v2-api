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
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/driftprogramming/pgxpoolmock"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
	db "github.com/stolostron/search-v2-api/pkg/database"
	"k8s.io/klog/v2"
)

var mux sync.RWMutex // Mutex to lock map during read/write

type SearchResult struct {
	input  *model.SearchInput
	pool   pgxpoolmock.PgxPool
	uids   []*string      // List of uids from search result to be used to get relatioinships.
	wg     sync.WaitGroup // WORKAROUND: Used to serialize search query and relatioinships query.
	query  string
	params []interface{}
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
	s.buildSearchQuery(context.Background(), true, false)
	count := s.resolveCount()

	return count
}

func (s *SearchResult) Items() []map[string]interface{} {
	s.wg.Add(1)
	defer s.wg.Done()
	klog.V(2).Info("Resolving SearchResult:Items()")
	s.buildSearchQuery(context.Background(), false, false)
	r, e := s.resolveItems()
	if e != nil {
		klog.Error("Error resolving items.", e)
	}
	return r
}

func (s *SearchResult) Related() []SearchRelatedResult {
	klog.V(2).Info("Resolving SearchResult:Related()")
	if s.uids == nil {
		s.Uids()
	}

	s.wg.Wait()
	var r []SearchRelatedResult

	if len(s.uids) > 0 {
		startTime := time.Now()
		r = s.getRelations()
		klog.Info("Time taken to resolve relationships: ", time.Since(startTime))
	} else {
		klog.Warning("No uids selected for query:Related()")
	}
	return r
}

func (s *SearchResult) Uids() {
	klog.V(2).Info("Resolving SearchResult:Uids()")
	s.buildSearchQuery(context.Background(), false, true)
	s.resolveUids()
}

func (s *SearchResult) buildSearchQuery(ctx context.Context, count bool, uid bool) {
	var limit int
	var selectDs *goqu.SelectDataset
	var whereDs []exp.Expression

	// Example query: SELECT uid, cluster, data FROM search.resources  WHERE lower(data->> 'kind') IN
	// (lower('Pod')) AND lower(data->> 'cluster') IN (lower('local-cluster')) LIMIT 10000

	//define schema table:
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)

	if s.input.Keywords != nil && len(s.input.Keywords) > 0 {
		jsb := goqu.L("jsonb_each_text(?)", goqu.C("data"))
		ds = goqu.From(schemaTable, jsb)
	}

	//SELECT CLAUSE
	if count {
		selectDs = ds.Select(goqu.COUNT("uid"))
	} else if uid {
		selectDs = ds.Select("uid")
	} else {
		selectDs = ds.Select("uid", "cluster", "data")
	}

	//WHERE CLAUSE
	if s.input != nil && (len(s.input.Filters) > 0 || (s.input.Keywords != nil && len(s.input.Keywords) > 0)) {
		whereDs = WhereClauseFilter(s.input)
	}

	//LIMIT CLAUSE
	if !count {
		limit = s.setLimit()
	}
	var params []interface{}
	var sql string
	var err error
	//Get the query
	if limit != 0 {
		sql, params, err = selectDs.Where(whereDs...).Limit(uint(limit)).ToSQL()
	} else {
		sql, params, err = selectDs.Where(whereDs...).ToSQL()
	}
	if err != nil {
		klog.Errorf("Error building Search query: %s", err.Error())
	}
	klog.V(3).Infof("Search query: %s\nargs: %s", sql, params)
	s.query = sql
	s.params = params
}

func (s *SearchResult) resolveCount() int {
	rows := s.pool.QueryRow(context.TODO(), s.query, s.params...)

	var count int
	err := rows.Scan(&count)
	if err != nil {
		klog.Errorf("Error %s resolving count for query:%s", err.Error(), s.query)
	}
	return count
}

func (s *SearchResult) resolveUids() {
	rows, err := s.pool.Query(context.Background(), s.query, s.params...)
	if err != nil {
		klog.Errorf("Error resolving query [%s] with args [%+v]. Error: [%+v]", s.query, s.params, err)
	}
	defer rows.Close()
	for rows.Next() {
		var uid string
		err = rows.Scan(&uid)
		if err != nil {
			klog.Errorf("Error %s retrieving rows for query:%s", err.Error(), s.query)
		}
		s.uids = append(s.uids, &uid)
	}

}
func (s *SearchResult) resolveItems() ([]map[string]interface{}, error) {
	items := []map[string]interface{}{}
	rows, err := s.pool.Query(context.Background(), s.query, s.params...)
	if err != nil {
		klog.Errorf("Error resolving query [%s] with args [%+v]. Error: [%+v]", s.query, s.params, err)
	}
	defer rows.Close()

	var cluster string
	var data map[string]interface{}
	s.uids = make([]*string, len(items))

	for rows.Next() {
		var uid string
		err = rows.Scan(&uid, &cluster, &data)
		if err != nil {
			klog.Errorf("Error %s retrieving rows for query:%s", err.Error(), s.query)
		}
		currItem := formatDataMap(data)
		currItem["_uid"] = uid
		currItem["cluster"] = cluster

		items = append(items, currItem)
		s.uids = append(s.uids, &uid)

	}

	return items, nil
}

//Set limit for queries
func (s *SearchResult) setLimit() int {
	var limit int
	if s.input != nil && s.input.Limit != nil && *s.input.Limit > 0 {
		limit = *s.input.Limit
	} else if s.input != nil && s.input.Limit != nil && *s.input.Limit == -1 {
		klog.Warning("No limit set. Fetching all results.")
	} else {
		limit = config.DEFAULT_QUERY_LIMIT
	}
	return limit
}

// Labels are sorted alphabetically to ensure consistency, then encoded in a
// string with the following format.
// key1:value1; key2:value2; ...
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
	return strings.Join(labelStrings, "; ")
}

// Encode array into a single string with the format.
//  value1; value2; ...
func formatArray(itemlist []interface{}) string {
	keys := make([]string, len(itemlist))
	for i, k := range itemlist {
		keys[i] = convertToString(k)
	}
	sort.Strings(keys)
	return strings.Join(keys, "; ")
}

// Convert interface to string format
func convertToString(data interface{}) string {
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
			item[key] = v //strings.ToLower(v)
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

func pointerToStringArray(pointerArray []*string) []string {

	values := make([]string, len(pointerArray))
	for i, val := range pointerArray {
		values[i] = *val
	}
	return values
}

func stringArrayToPointer(stringArray []string) []*string {

	values := make([]*string, len(stringArray))
	for i, val := range stringArray {
		tmpVal := val
		values[i] = &tmpVal
	}
	return values
}
func WhereClauseFilter(input *model.SearchInput) []exp.Expression {
	var whereDs []exp.Expression

	if input.Keywords != nil && len(input.Keywords) > 0 {
		//query example: SELECT COUNT("uid") FROM "search"."resources", jsonb_each_text("data") WHERE (("value" LIKE '%dns%') AND ("data"->>'kind' IN ('Pod')))
		keywords := pointerToStringArray(input.Keywords)
		for _, key := range keywords {
			key = "%" + key + "%"
			whereDs = append(whereDs, goqu.L(`"value"`).Like(key).Expression())
		}
	}
	if input.Filters != nil {
		for _, filter := range input.Filters {
			if len(filter.Values) > 0 {
				values := pointerToStringArray(filter.Values)

				if filter.Property == "cluster" {
					whereDs = append(whereDs, goqu.C(filter.Property).In(values).Expression())
				} else {
					whereDs = append(whereDs, goqu.L(`"data"->>?`, filter.Property).In(values).Expression())
				}
			} else {
				klog.Warningf("Ignoring filter [%s] because it has no values", filter.Property)
			}
		}
	}

	return whereDs
}
