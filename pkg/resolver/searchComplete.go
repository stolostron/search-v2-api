package resolver

import (
	"context"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/driftprogramming/pgxpoolmock"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
	db "github.com/stolostron/search-v2-api/pkg/database"
	klog "k8s.io/klog/v2"
)

type SearchCompleteResult struct {
	input    *model.SearchInput
	pool     pgxpoolmock.PgxPool
	property string
	limit    *int
	query    string
	params   []interface{}
}

func (s *SearchCompleteResult) autoComplete(ctx context.Context) ([]*string, error) {
	s.searchCompleteQuery(ctx)
	res, autoCompleteErr := s.searchCompleteResults(ctx)
	if autoCompleteErr != nil {
		klog.Error("Error resolving properties in autoComplete", autoCompleteErr)
	}
	return res, autoCompleteErr
}

func SearchComplete(ctx context.Context, property string, srchInput *model.SearchInput, limit *int) ([]*string, error) {

	searchCompleteResult := &SearchCompleteResult{
		input:    srchInput,
		pool:     db.GetConnection(),
		property: property,
		limit:    limit,
	}
	return searchCompleteResult.autoComplete(ctx)

}

func (s *SearchCompleteResult) searchCompleteQuery(ctx context.Context) {
	var limit int
	var whereDs []exp.Expression
	var selectDs *goqu.SelectDataset

	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)
	if s.property != "" {
		//WHERE CLAUSE
		if s.input != nil && len(s.input.Filters) > 0 {
			whereDs = WhereClauseFilter(s.input)
		}
		//SELECT CLAUSE
		if s.property == "cluster" {
			selectDs = ds.SelectDistinct(s.property).Order(goqu.C(s.property).Asc())
			//Adding notNull clause to filter out NULL values and ORDER by sort results
			whereDs = append(whereDs, goqu.C(s.property).IsNotNull())
			whereDs = append(whereDs, goqu.C(s.property).Neq(""))
		} else {
			selectDs = ds.SelectDistinct(goqu.L(`"data"->>?`, s.property)).Order(goqu.L(`"data"->>?`, s.property).Asc())
			//Adding notNull clause to filter out NULL values and ORDER by sort results
			whereDs = append(whereDs, goqu.L(`"data"->>?`, s.property).IsNotNull())
		}
		//LIMIT CLAUSE
		if s.input != nil && s.input.Limit != nil && *s.input.Limit > 0 {
			limit = *s.input.Limit
		} else if s.input != nil && s.input.Limit != nil && *s.input.Limit == -1 {
			klog.Warning("No limit set. Fetching all results.")
		} else {
			limit = config.Cfg.QueryLimit
		}
		//Get the query
		sql, params, err := selectDs.Where(whereDs...).Limit(uint(limit)).ToSQL()
		if err != nil {
			klog.Errorf("Error building SearchComplete query: %s", err.Error())
		}
		s.query = sql
		s.params = params
		klog.V(3).Info("SearchComplete Query: ", s.query)
	} else {
		s.query = ""
		s.params = nil
	}

}

func (s *SearchCompleteResult) searchCompleteResults(ctx context.Context) ([]*string, error) {
	klog.V(2).Info("Resolving searchCompleteResults()")
	rows, err := s.pool.Query(ctx, s.query, s.params...)
	if err != nil {
		klog.Error("Error fetching search complete results from db ", err)
	}
	defer rows.Close()
	var srchCompleteOut []*string
	if rows != nil {
		for rows.Next() {
			prop := ""
			scanErr := rows.Scan(&prop)
			if scanErr != nil {
				klog.Info("Error reading searchCompleteResults", scanErr)
			}
			srchCompleteOut = append(srchCompleteOut, &prop)
		}
	}
	return srchCompleteOut, nil
}
