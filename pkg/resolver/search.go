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

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/lib/pq"
	"github.com/stolostron/search-v2-api/graph/model"
	db "github.com/stolostron/search-v2-api/pkg/database"
	"k8s.io/klog/v2"
)

type SearchResult struct {
	input *model.SearchInput
	pool  pgxpoolmock.PgxPool
	uids  []*string      // List of uids from search result to be used to get relatioinships.
	wg    sync.WaitGroup // WORKAROUND: Used to serialize search query and relatioinships query.
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
	qString, qArgs := s.buildSearchQuery(context.Background(), true, false)
	count, e := s.resolveCount(qString, qArgs)

	if e != nil {
		klog.Error("Error resolving count.", e)
	}
	return count
}

// func (s *SearchResult) Uids() error {
// 	qString, qArgs := s.buildSearchQuery(context.Background(), false, true)
// 	e := s.resolveUids(qString, qArgs)

// 	if e != nil {
// 		klog.Error("Error resolving uids.", e)
// 	}
// 	return e
// }

func (s *SearchResult) Items() []map[string]interface{} {
	s.wg.Add(1)
	defer s.wg.Done()
	klog.Info("Resolving SearchResult:Items()")
	qString, qArgs := s.buildSearchQuery(context.Background(), false, false)
	r, e := s.resolveItems(qString, qArgs)
	if e != nil {
		klog.Error("Error resolving items.", e)
	}
	return r
}

func (s *SearchResult) Related() []SearchRelatedResult {
	klog.Info("Resolving SearchResult:Related()")

	// FIXME: WORKAROUND when the query doesn't request Items() we must use a more efficient query to get the uids.
	if s.uids == nil {
		klog.Warning("TODO: Use a query that only selects the UIDs.")
		s.Items()
	}
	s.wg.Wait() // WORKAROUND wait to complete execution of Items()

	r := s.getRelations()
	return r
}

//=====================

var trimAND string = " AND "

func (s *SearchResult) buildSearchQuery(ctx context.Context, count bool, uid bool) (string, []interface{}) {
	var selectClause, whereClause, limitClause, limitStr, query string
	var args []interface{}
	// SELECT uid, cluster, data FROM search.resources  WHERE lower(data->> 'kind') IN (lower('Pod')) AND lower(data->> 'cluster') IN (lower('local-cluster')) LIMIT 10000
	selectClause = "SELECT uid, cluster, data FROM search.resources "
	if count {
		selectClause = "SELECT count(uid) FROM search.resources "
	}
	if uid {
		selectClause = "SELECT uid FROM search.resources"
	}

	whereClause = " WHERE "

	for i, filter := range s.input.Filters {
		klog.Infof("Filters%d: %+v", i, *filter)
		// TODO: Handle other column names like kind and namespace
		if filter.Property == "cluster" {
			whereClause = whereClause + filter.Property
		} else {
			// TODO: To be removed when indexer handles this as adding lower hurts index scans
			whereClause = whereClause + "lower(data->> '" + filter.Property + "')"
		}
		var values []string

		if len(filter.Values) > 1 {
			for _, val := range filter.Values {
				klog.Infof("Filter value: %s", *val)
				values = append(values, strings.ToLower(*val))
				//TODO: Here, assuming value is string. Check for other cases.
				//TODO: Remove lower() conversion once data is correctly loaded from indexer
				// "SELECT id FROM search.resources WHERE status = any($1)"
				//SELECT id FROM search.resources WHERE status = ANY('{"Running", "Error"}');
			}
			whereClause = whereClause + "=any($" + strconv.Itoa(i+1) + ") AND "
			args = append(args, pq.Array(values))
		} else if len(filter.Values) == 1 {
			whereClause = whereClause + "=$" + strconv.Itoa(i+1) + " AND "
			val := filter.Values[0]
			args = append(args, strings.ToLower(*val))
		}
	}
	if s.input.Limit != nil {
		limitStr = strconv.Itoa(*s.input.Limit)
	}
	if limitStr != "" {
		limitClause = " LIMIT " + limitStr
		query = selectClause + strings.TrimRight(whereClause, trimAND) + limitClause

	} else {
		query = selectClause + strings.TrimRight(whereClause, trimAND)
	}
	klog.Infof("query: %s\nargs: %+v", query, args)

	return query, args
}

func (s *SearchResult) resolveCount(query string, args []interface{}) (int, error) {
	rows := s.pool.QueryRow(context.Background(), query, args...)

	var count int
	err := rows.Scan(&count)

	return count, err
}

// func (s *SearchResult) resolveUids(query string, args []interface{}) error {
// 	rows, err := s.pool.Query(context.Background(), query, args...)
// 	if err != nil {
// 		klog.Errorf("Error resolving query [%s] with args [%+v]. Error: [%+v]", query, args, err)
// 	}
// 	var uid string

// 	defer rows.Close()
// 	for rows.Next() {
// 		err = rows.Scan(&uid)
// 		if err != nil {
// 			klog.Errorf("Error %s retrieving rows for query:%s", err.Error(), query)
// 		}
// 		s.uids = append(s.uids, &uid)
// 	}
// 	return err

// }

func (s *SearchResult) resolveItems(query string, args []interface{}) ([]map[string]interface{}, error) {
	rows, err := s.pool.Query(context.Background(), query, args...)
	if err != nil {
		klog.Errorf("Error resolving query [%s] with args [%+v]. Error: [%+v]", query, args, err)
	}
	defer rows.Close()

	// var uid string
	var cluster string
	var data map[string]interface{}
	items := []map[string]interface{}{}
	s.uids = make([]*string, len(items))

	for rows.Next() {
		var uid string
		err = rows.Scan(&uid, &cluster, &data)
		if err != nil {
			klog.Errorf("Error %s retrieving rows for query:%s", err.Error(), query)
		}

		currItem := formatDataMap(data)
		currItem["_uid"] = uid
		currItem["cluster"] = cluster

		items = append(items, currItem)
		s.uids = append(s.uids, &uid)
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
	var relQuery string

	// LEARNING: IN is equivalent to = ANY and performance is not deteriorated when we replace IN with =ANY
	relQuery = strings.TrimSpace(`WITH RECURSIVE
	search_graph(uid, data, destkind, sourceid, destid, path, level)
	AS (
	SELECT r.uid, r.data, e.destkind, e.sourceid, e.destid, ARRAY[r.uid] AS path, 1 AS level
		FROM search.resources r
		INNER JOIN
			search.edges e ON (r.uid = e.sourceid) OR (r.uid = e.destid)
		 WHERE r.uid = ANY($1)
	UNION
	SELECT r.uid, r.data, e.destkind, e.sourceid, e.destid, path||r.uid, level+1 AS level
		FROM search.resources r
		INNER JOIN
			search.edges e ON (r.uid = e.sourceid)
		, search_graph sg
		WHERE (e.sourceid = sg.destid OR e.destid = sg.sourceid)
		AND r.uid <> all(sg.path)
		AND level = 1
		)
	SELECT distinct ON (destid) data, destid, destkind FROM search_graph WHERE level=1 OR destid = ANY($1)`)

	klog.Info("LENGTH OF QUERY:", len(relQuery))
	klog.Info("NUMBER OF UIDS IN RELATED:", len(s.uids))

	relations, QueryError := s.pool.Query(context.Background(), relQuery, s.uids) // how to deal with defaults.
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

		break
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

	fmt.Println("LENGTH OF REALTEDSEARCH:\n", len(relatedSearch))
	fmt.Println("related kind:", relatedSearch[0].Kind)

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
		default:
			klog.Warningf("Error formatting property with key: %+v  type: %+v\n", key, reflect.TypeOf(v))
			continue
		}
	}
	return item
}
