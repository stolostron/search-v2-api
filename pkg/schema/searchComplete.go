package schema

import (
	"context"
	"fmt"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
	db "github.com/stolostron/search-v2-api/pkg/database"
	res "github.com/stolostron/search-v2-api/pkg/resolver"
	klog "k8s.io/klog/v2"
)

type SearchCompleteResult struct {
	input    *model.SearchInput
	pool     pgxpoolmock.PgxPool
	property string
	limit    *int
}

func (s *SearchCompleteResult) autoComplete(ctx context.Context) ([]*string, error) {
	query, args := s.searchCompleteQuery(ctx)
	res, autoCompleteErr := s.searchCompleteResults(query, args)
	if autoCompleteErr != nil {
		klog.Error("Error resolving properties in autoComplete", autoCompleteErr)
	}
	return res, autoCompleteErr
}

func SearchComplete(ctx context.Context, property string, srchInput *model.SearchInput, limit *int) ([]*string, error) {
	var searchCompleteResult *SearchCompleteResult

	if srchInput != nil {
		searchCompleteResult = &SearchCompleteResult{
			input:    srchInput,
			pool:     db.GetConnection(),
			property: property,
			limit:    limit,
		}
	}
	return searchCompleteResult.autoComplete(ctx)

}

func (s *SearchCompleteResult) searchCompleteQuery(ctx context.Context) (string, []interface{}) {
	var selectClause, whereClause, notNullClause, query string
	var limit int
	var args []interface{}
	argCount := 1

	whereClause, args, argCount = res.WhereClauseFilter(args, s.input, argCount)

	if s.property != "" {
		if s.property == "cluster" {
			//Adding notNull clause to filter out NULL values and ORDER by sort results
			selectClause = fmt.Sprintf("%s $%d %s", "SELECT DISTINCT ", argCount, " FROM search.resources")
			notNullClause = fmt.Sprintf("$%d %s $%d", argCount, " IS NOT NULL ORDER BY ", argCount)
			args = append(args, s.property)
			argCount++
		} else {
			//Adding notNull clause to filter out NULL values and ORDER by sort results
			selectClause = fmt.Sprintf("%s '$%d' %s", "SELECT DISTINCT data->>", argCount, " FROM search.resources")
			notNullClause = fmt.Sprintf("data->>'$%d' %s data->>'$%d'", argCount, " IS NOT NULL ORDER BY ", argCount)
			args = append(args, s.property)
			argCount++
		}
		if s.input.Limit != nil && *s.input.Limit != 0 {
			limit = *s.input.Limit
		} else {
			limit = config.DEFAULT_QUERY_LIMIT
		}
		args = append(args, limit)
		query = fmt.Sprintf("%s %s %s LIMIT $%d", selectClause, whereClause, notNullClause, argCount)
		klog.Infof("SearchComplete: %s\nargs: %s", query, args)
		return query, args
	}

	return "", nil
}

func (s *SearchCompleteResult) searchCompleteResults(query string, args []interface{}) ([]*string, error) {

	//TODO: Handle error
	rows, err := s.pool.Query(context.Background(), query, args...)
	if err != nil {
		klog.Error("Error fetching results from db ", err)
	}
	defer rows.Close()
	var srchCompleteOut []*string
	var prop string
	for rows.Next() {
		scanErr := rows.Scan(&prop)
		if scanErr != nil {
			klog.Info("Error reading searchCompleteResults", scanErr)
		}
		tmpProp := prop
		fmt.Println("Current prop is: ", tmpProp)
		srchCompleteOut = append(srchCompleteOut, &tmpProp)
	}
	return srchCompleteOut, nil
}
