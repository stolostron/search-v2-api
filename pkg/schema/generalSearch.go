package schema

import (
	"context"
	"strconv"
	"strings"

	klog "k8s.io/klog/v2"

	"github.com/SherinV/search-api/graph/model"
	db "github.com/SherinV/search-api/pkg/database"
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
	}
	klog.Info("len items: ", len(items))
	totalCount := len(items)
	srchrelatedresult := make([]*model.SearchRelatedResult, 0)
	nodecount := 2
	clustercount := 1

	srchrelatedresult1 := model.SearchRelatedResult{Kind: "Node", Count: &nodecount}
	srchrelatedresult2 := model.SearchRelatedResult{Kind: "Cluster", Count: &clustercount}
	srchrelatedresult = append(srchrelatedresult, &srchrelatedresult1)
	srchrelatedresult = append(srchrelatedresult, &srchrelatedresult2)

	srchresult1 := model.SearchResult{
		Count:   &totalCount,
		Items:   items,
		Related: srchrelatedresult,
	}
	return &srchresult1, nil
}
