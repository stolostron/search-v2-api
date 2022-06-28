package resolver

import (
	"context"
	"sort"
	"strconv"
	"time"

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
		klog.Error("Error resolving properties in autoComplete. ", autoCompleteErr)
	}
	klog.Info("Returning searchCompleteResult")
	return res, autoCompleteErr
}

func SearchComplete(ctx context.Context, property string, srchInput *model.SearchInput, limit *int) ([]*string, error) {
	klog.Info("In SearchComplete. Creating searchCompleteResult struct")

	searchCompleteResult := &SearchCompleteResult{
		input:    srchInput,
		pool:     db.GetConnection(),
		property: property,
		limit:    limit,
	}
	klog.Info("Created searchCompleteResult struct")
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
			whereDs = append(whereDs, goqu.C(s.property).IsNotNull(),
				goqu.C(s.property).Neq("")) // remove empty strings from results
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
	srchCompleteOut := make([]*string, 0)

	if err != nil {
		klog.Error("Error fetching search complete results from db ", err)
		return srchCompleteOut, err
	}
	defer rows.Close()
	if rows != nil {
		for rows.Next() {
			prop := ""
			scanErr := rows.Scan(&prop)
			if scanErr != nil {
				klog.Error("Error reading searchCompleteResults", scanErr)
			}
			srchCompleteOut = append(srchCompleteOut, &prop)
		}
	}
	isNumber := isNumber(srchCompleteOut)
	if isNumber { //check if valid number
		isNumber := "isNumber"
		srchCompleteOutNum := []*string{&isNumber} //isNumber should be the first argument if the property is a number
		// Sort the values in srchCompleteOut
		sort.Slice(srchCompleteOut, func(i, j int) bool {
			numA, _ := strconv.Atoi(*srchCompleteOut[i])
			numB, _ := strconv.Atoi(*srchCompleteOut[j])
			return numA < numB
		})
		if len(srchCompleteOut) > 1 {
			// Pass only the min and max values of the numbers to show the range in the UI
			srchCompleteOut = append(srchCompleteOutNum, srchCompleteOut[0], srchCompleteOut[len(srchCompleteOut)-1])
		} else {
			srchCompleteOut = append(srchCompleteOutNum, srchCompleteOut...)
		}

	}
	if !isNumber && isDate(srchCompleteOut) { //check if valid date
		isDate := "isDate"
		srchCompleteOutNum := []*string{&isDate}
		srchCompleteOut = srchCompleteOutNum
	}

	return srchCompleteOut, nil
}

// check if a given string is of type date
func isDate(vals []*string) bool {
	for _, val := range vals {
		// parse string date to golang time format: YYYY-MM-DDTHH:mm:ssZ i.e. "2022-01-01T17:17:09Z"
		// const time.RFC3339 is YYYY-MM-DDTHH:mm:ssZ format ex:"2006-01-02T15:04:05Z07:00"

		if _, err := time.Parse(time.RFC3339, *val); err != nil {
			return false
		}
	}
	return true
}

// check if a given string is of type number (int)
func isNumber(vals []*string) bool {

	for _, val := range vals {
		if _, err := strconv.Atoi(*val); err != nil {
			return false
		}
	}
	return true
}
