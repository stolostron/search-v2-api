package schema

import (
	"context"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
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
	var limit int
	var whereDs []exp.Expression
	var selectDs *goqu.SelectDataset

	//FROM CLAUSE
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)
	//WHERE CLAUSE
	whereDs = res.WhereClauseFilter(s.input)
	//SELECT CLAUSE
	if s.property != "" {
		if s.property == "cluster" {
			selectDs = ds.SelectDistinct(s.property).Order(goqu.C(s.property).Desc())
			//Adding notNull clause to filter out NULL values and ORDER by sort results
			whereDs = append(whereDs, goqu.C(s.property).IsNotNull())
		} else {
			selectDs = ds.SelectDistinct(goqu.L(`"data"->>?`, s.property)).Order(goqu.L(`"data"->>?`, s.property).Desc())
			//Adding notNull clause to filter out NULL values and ORDER by sort results
			whereDs = append(whereDs, goqu.L(`"data"->>?`, s.property).IsNotNull())
		}
		//LIMIT CLAUSE
		if s.input.Limit != nil && *s.input.Limit != 0 {
			limit = *s.input.Limit
		} else {
			limit = config.DEFAULT_QUERY_LIMIT
		}
		//Get the query
		sql, params, err := selectDs.Where(whereDs...).Limit(uint(limit)).ToSQL()
		if err != nil {
			klog.Infof("Error building SearchComplete query: %s", err.Error())
		}
		klog.Infof("SearchComplete: %s\nargs: %s", sql, params)
		return sql, params
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
		srchCompleteOut = append(srchCompleteOut, &tmpProp)
	}
	return srchCompleteOut, nil
}
