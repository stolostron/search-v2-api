package resolver

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/lib/pq"
	"github.com/stolostron/search-v2-api/graph/model"
	db "github.com/stolostron/search-v2-api/pkg/database"
	"k8s.io/klog/v2"
)

type SearchResult struct {
	input *model.SearchInput
	pool  pgxpoolmock.PgxPool
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
	qString, qArgs := s.buildSearchQuery(context.Background(), true)
	count, e := s.resolveCount(qString, qArgs)

	if e != nil {
		klog.Error("Error resolving count.", e)
	}
	return count
}

func (s *SearchResult) Items() []map[string]interface{} {
	qString, qArgs := s.buildSearchQuery(context.Background(), false)
	r, e := s.resolveItems(qString, qArgs)
	if e != nil {
		klog.Error("Error resolving items.", e)
	}
	return r
}

func (s *SearchResult) Related() []SearchRelatedResult {
	fmt.Printf("Resolving SearchResult:Related() - input: %+v\n", s.input)
	r := make([]SearchRelatedResult, 1)
	return r
}

//=====================

var trimAND string = " AND "

func (s *SearchResult) buildSearchQuery(ctx context.Context, count bool) (string, []interface{}) {
	var selectClause, whereClause, limitClause, limitStr, query string
	var args []interface{}
	// SELECT uid, cluster, data FROM resources  WHERE lower(data->> 'kind') IN (lower('Pod')) AND lower(data->> 'cluster') IN (lower('local-cluster')) LIMIT 10000
	selectClause = "SELECT uid, cluster, data FROM resources "
	if count {
		selectClause = "SELECT count(uid) FROM resources "
	}
	limitClause = " LIMIT "

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

func (s *SearchResult) resolveItems(query string, args []interface{}) ([]map[string]interface{}, error) {
	rows, _ := s.pool.Query(context.Background(), query, args...)
	//TODO: Handle error
	defer rows.Close()
	var uid, cluster string
	var data map[string]interface{}
	items := []map[string]interface{}{}

	for rows.Next() {
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

	return items, nil
}
