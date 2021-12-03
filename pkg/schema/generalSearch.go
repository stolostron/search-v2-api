package schema

import (
	"context"
	"fmt"

	// "fmt"
	"strconv"
	"strings"

	klog "k8s.io/klog/v2"

	"github.com/SherinV/search-api/graph/model"
	db "github.com/SherinV/search-api/pkg/database"

	// "github.com/jackc/pgx/v4"
	"github.com/lib/pq"
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
	uidArray := make([]string, 0, len(items))

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
		fmt.Println(uidArray)
	}
	klog.Info("len items: ", len(items))
	totalCount := len(items)

	srchrelatedresult := getRelations(uidArray)

	srchresult1 := model.SearchResult{
		Count:   &totalCount,
		Items:   items,
		Related: srchrelatedresult,
	}
	return &srchresult1, nil
}

func getRelations(uidArray []string) model.SearchRelatedResult {

	pool := db.GetConnection()
	relations, _ := pool.Query(context.Background(),
		// relations, err  := pool.Query(context.Background(),
		`with recursive
	search_graph(uid, data, sourcekind, destkind, sourceid, destid, path, level)
	as (
	SELECT r.uid, r.data, e.sourcekind, e.destkind, e.sourceid, e.destid, ARRAY[r.uid] as path, 1 as level
		from resources r
		INNER JOIN
			edges e ON (r.uid = e.sourceid)
		 where r.uid in ("%s")
	union
	select r.uid, r.data, e.sourcekind, e.destkind, e.sourceid, e.destid, path||r.uid, level+1 as level
		from resources r
		INNER JOIN
			edges e ON (r.uid = e.sourceid)
		, search_graph sg
		where (e.sourceid = sg.destid or e.destid = sg.sourceid)
		and r.uid <> all(sg.path)
		)

	select data, destid, destkind from search_graph where level= 1 or destid in ("%s")`, uidArray)

	//destid = uid of related resource,
	//destkind = kind of related resource,
	//count = count of related resource

	defer relations.Close()

	var destkind, destid string
	var data map[string]interface{}
	items := []map[string]interface{}{}

	for relations.Next() {
		relatedResultError := relations.Scan(&destid, &destkind)
		if relatedResultError != nil {
			klog.Errorf("Error %s retrieving rows for relationships:%s", relatedResultError.Error(), relations)
		}
	}
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
	currdestUid := destid
	currItem["destid"] = currdestUid
	currdestKind := destkind
	currItem["destkind"] = currdestKind
	items = append(items, currItem)

	klog.Info("len items: ", len(items))
	totalCount := len(items)

	relatedSearch := model.SearchRelatedResult{

		Kind:  currdestKind,
		Count: &totalCount,
		Items: items,
	}
	return relatedSearch
}
