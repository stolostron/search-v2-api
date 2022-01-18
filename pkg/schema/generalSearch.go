package schema

import (
	"context"
	"strconv"
	"strings"

	klog "k8s.io/klog/v2"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/lib/pq"
	"github.com/open-cluster-management/search-v2-api/graph/model"
	db "github.com/open-cluster-management/search-v2-api/pkg/database"
)

var trimAND string = " AND "

func Search(ctx context.Context, input []*model.SearchInput) ([]*model.SearchResult, error) {
	limit := 0
	srchResult := make([]*model.SearchResult, 0)

	if len(input) > 0 {
		for _, in := range input {
			query, args := searchQuery(ctx, in, &limit)
			klog.Infof("Search Query:", query)
			//TODO: Check error
			srchRes, _ := searchResults(query, args)
			srchResult = append(srchResult, srchRes)
		}
	}
	return srchResult, nil
}

func searchQuery(ctx context.Context, input *model.SearchInput, limit *int) (string, []interface{}) {
	var selectClause, whereClause, limitClause, limitStr, query string
	var args []interface{}
	// SELECT uid, cluster, data FROM resources  WHERE lower(data->> 'kind') IN (lower('Pod')) AND lower(data->> 'cluster') IN (lower('local-cluster')) LIMIT 10000
	selectClause = "SELECT uid, cluster, data FROM resources "
	limitClause = " LIMIT "

	whereClause = " WHERE "

	for i, filter := range input.Filters {
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
				// "SELECT id FROM resources WHERE status = any($1)"
				//SELECT id FROM resources WHERE status = ANY('{"Running", "Error"}');
			}
			whereClause = whereClause + "=any($" + strconv.Itoa(i+1) + ") AND "
			args = append(args, pq.Array(values))
		} else if len(filter.Values) == 1 {
			whereClause = whereClause + "=$" + strconv.Itoa(i+1) + " AND "
			val := filter.Values[0]
			args = append(args, strings.ToLower(*val))
		}
	}
	if input.Limit != nil {
		limitStr = strconv.Itoa(*input.Limit)
	}
	if limitStr != "" {
		limitClause = " LIMIT " + limitStr
		query = selectClause + strings.TrimRight(whereClause, trimAND) + limitClause

	} else {
		query = selectClause + strings.TrimRight(whereClause, trimAND)
	}
	klog.Infof("args: %+v", args)

	return query, args
}

func searchResults(query string, args []interface{}) (*model.SearchResult, error) {

	pool := db.GetConnection()
	rows, _ := pool.Query(context.Background(), query, args...)
	//TODO: Handle error
	defer rows.Close()
	var uid, cluster string
	var data map[string]interface{}
	items := []map[string]interface{}{}
	//used for getRelations function:
	uidArray := make([]string, 0, len(items))
	// var level int

	for rows.Next() {

		// rowValues, _ := rows.Values()
		err := rows.Scan(&uid, &cluster, &data)
		if err != nil {
			klog.Errorf("Error %s retrieving rows for query:%s", err.Error(), query)
		}

		// TODO: To be removed when indexer handles this. Currently only string type is handled.
		currItem := make(map[string]interface{})
		for k, myInterface := range data {
			switch v := myInterface.(type) {
			case string:
				currItem[k] = strings.ToLower(v)
			default:
				// klog.Info("Not string type.", k, v)
				continue
			}

		}
		currUid := uid
		currItem["_uid"] = currUid
		currCluster := cluster
		currItem["cluster"] = currCluster
		items = append(items, currItem)

		uidArray = append(uidArray, currUid)

	}
	klog.Info("len search result items: ", len(items))
	totalCount := len(items)

	srchrelatedresult := getRelations(pool, uidArray)
	for i, result := range srchrelatedresult {
		klog.Infof("THIS IS VARIABLE: %d %d %s", i, result.Count, result.Kind)

	}

	klog.Infof("%+v\n", srchrelatedresult)
	srchresult1 := model.SearchResult{
		Count:   &totalCount,
		Items:   items,
		Related: srchrelatedresult,
	}
	return &srchresult1, nil
}

func getRelations(pool *pgxpool.Pool, uidArray []string) []*model.SearchRelatedResult {

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
		from resources r
		INNER JOIN
			edges e ON (r.uid = e.sourceid) or (r.uid = e.destid)
		 where r.uid = ANY($1)
	union
	select r.uid, r.data, e.sourcekind, e.destkind, e.sourceid, e.destid, path||r.uid, level+1 as level
		from resources r
		INNER JOIN
			edges e ON (r.uid = e.sourceid)
		, search_graph sg
		where (e.sourceid = sg.destid or e.destid = sg.sourceid)
		and r.uid <> all(sg.path)
		and level = 1 
		)
	select distinct on (destid) data, destid, destkind from search_graph where level=1 or destid = ANY($2)`

	relations, QueryError := pool.Query(context.Background(), recrusiveQuery, uidArray, uidArray) // how to deal with defaults.
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
		currItem := make(map[string]interface{})
		for k, myInterface := range data {
			switch v := myInterface.(type) {
			case string:
				currItem[k] = strings.ToLower(v)
			default:
				// klog.Info("Not string type.", k, v)
				continue
			}
		}
		// creating currKind variable to store kind and appending to list
		currKind := destkind
		currItem["Kind"] = currKind
		kindSlice = append(kindSlice, currKind)
		items = append(items, currItem)

	}
	// saving relationships:
	// json_items, _ := json.Marshal(items)
	// fmt.Println("All related results", string(json_items))
	// ioutil.WriteFile("rel_data.json", json_items, os.ModePerm)

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
	relatedSearch := make([]*model.SearchRelatedResult, len(count))

	//iterating and sending values to relatedSearch
	for i := range kindList {
		kind := kindList[i]
		count := countList[i]
		relatedSearch[i] = &model.SearchRelatedResult{kind, &count, items}
	}

	return relatedSearch
}

//helper function TODO: make helper.go module to store these if needed.
func printUniqueValue(arr []string) map[string]int {
	//Create a   dictionary of values for each element
	dict := make(map[string]int)
	for _, num := range arr {
		dict[num] = dict[num] + 1
	}
	return dict
}
