package resolver

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/driftprogramming/pgxpoolmock"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
	db "github.com/stolostron/search-v2-api/pkg/database"
	"github.com/stolostron/search-v2-api/pkg/metrics"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	klog "k8s.io/klog/v2"
)

type SearchCompleteResult struct {
	input     *model.SearchInput
	pool      pgxpoolmock.PgxPool
	property  string
	limit     *int
	query     string
	params    []interface{}
	propTypes map[string]string
	userData  rbac.UserData
}

var arrayProperties = make(map[string]struct{})

func (s *SearchCompleteResult) autoComplete(ctx context.Context) ([]*string, error) {
	s.searchCompleteQuery(ctx)
	res, autoCompleteErr := s.searchCompleteResults(ctx)
	if autoCompleteErr != nil {
		klog.Error("Error resolving properties in autoComplete. ", autoCompleteErr)
	}
	return res, autoCompleteErr
}

func SearchComplete(ctx context.Context, property string, srchInput *model.SearchInput, limit *int) ([]*string, error) {
	defer metrics.SlowLog("SearchCompleteResolver", 0)()
	userData, userDataErr := rbac.GetCache().GetUserData(ctx)
	if userDataErr != nil {
		return []*string{}, userDataErr
	}

	// Check that shared cache has property types:
	propTypes, err := rbac.GetCache().GetPropertyTypes(ctx, false)
	if err != nil {
		klog.Warningf("Error creating datatype map with err: [%s] ", err)
	}

	// Proceed if user's rbac data exists
	searchCompleteResult := &SearchCompleteResult{
		input:     srchInput,
		pool:      db.GetConnPool(ctx),
		property:  property,
		limit:     limit,
		userData:  userData,
		propTypes: propTypes,
	}
	return searchCompleteResult.autoComplete(ctx)

}

// Sample query: SELECT DISTINCT name FROM
// (SELECT "data"->>'name' as name FROM "search"."resources" WHERE ("data"->>'name' IS NOT NULL)
// LIMIT 100000) as searchComplete
// ORDER BY name ASC
// LIMIT 1000
func (s *SearchCompleteResult) searchCompleteQuery(ctx context.Context) {
	var limit uint
	var whereDs []exp.Expression
	var selectDs *goqu.SelectDataset

	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)
	if s.property != "" {

		// WHERE CLAUSE
		if s.input != nil && len(s.input.Filters) > 0 {
			whereDs, s.propTypes, _ = WhereClauseFilter(ctx, s.input, s.propTypes)
		}

		// SELECT CLAUSE
		if s.property == "cluster" {
			selectDs = ds.SelectDistinct(goqu.C(s.property)).Order(goqu.C(s.property).Asc())
			//Adding notNull clause to filter out NULL values and ORDER by sort results
			whereDs = append(whereDs, goqu.C(s.property).IsNotNull(),
				goqu.C(s.property).Neq("")) // remove empty strings from results
		} else {
			// "->" - get data as json object
			// "->>" - get data as string
			selectDs = ds.SelectDistinct(goqu.L(`"data"->?`, s.property)).Order(goqu.L(`"data"->?`, s.property).Asc())
			//Adding notNull clause to filter out NULL values and ORDER by sort results
			whereDs = append(whereDs, goqu.L(`"data"->?`, s.property).IsNotNull())
		}

		// get user info for logging
		_, userInfo := rbac.GetCache().GetUserUID(ctx)

		// RBAC CLAUSE
		// if one of them is not nil, userData is not empty
		if s.userData.CsResources != nil || s.userData.NsResources != nil || s.userData.ManagedClusters != nil {
			whereDs = append(whereDs,
				buildRbacWhereClause(ctx, s.userData, userInfo)) // add rbac
		} else {
			klog.Errorf("Error building searchComplete query: RBAC clause is required!"+
				" None found for searchComplete query %+v for user %s with uid %s ",
				s.input, userInfo.Username, userInfo.UID)

			s.query = ""
			s.params = nil
			return
		}

		// LIMIT CLAUSE
		if s.limit != nil && *s.limit > 0 {
			limit = uint(*s.limit)
		} else if s.limit != nil && *s.limit == -1 {
			klog.Warning("Limit set to -1. Fetching all results. This may affect performance.")
		} else {
			limit = config.Cfg.QueryLimit
		}

		var params []interface{}
		var sql string
		var err error

		// Get the query
		if limit > 0 {
			sql, params, err = selectDs.Where(whereDs...).Limit(limit).ToSQL()
		} else {
			sql, params, err = selectDs.Where(whereDs...).ToSQL()
		}

		if err != nil {
			klog.Errorf("Error building SearchComplete query: %s", err.Error())
		}
		s.query = sql
		s.params = params
		klog.V(5).Info("SearchComplete Query: ", s.query)
	} else {
		s.query = ""
		s.params = nil
	}
	// SELECT DISTINCT "prop" FROM (SELECT "data"->'?'
	// AS "prop" FROM "search"."resources" WHERE ("data"->'?' IS NOT NULL) LIMIT 100000)
	// AS "searchComplete" ORDER BY prop ASC LIMIT 1000

}

func (s *SearchCompleteResult) searchCompleteResults(ctx context.Context) ([]*string, error) {
	klog.V(2).Info("Resolving searchCompleteResults()")
	rows, err := s.pool.Query(ctx, s.query, s.params...)
	srchCompleteOut := make([]*string, 0)

	if err != nil {
		klog.Error("Error fetching search complete results from db ", err)
		return srchCompleteOut, err
	}

	if rows != nil {
		defer rows.Close()
		props := make(map[string]struct{})
		for rows.Next() {
			prop := ""
			var input interface{}
			scanErr := rows.Scan(&input)
			if scanErr != nil {
				klog.Error("Error reading searchCompleteResults", scanErr)
			}

			switch v := input.(type) {
			case string:
				prop = v
				props[v] = struct{}{}
			case bool:
				prop = strconv.FormatBool(v)
				props[prop] = struct{}{}
			case float64:
				prop = strconv.FormatInt(int64(v), 10)
				props[prop] = struct{}{}
			case map[string]interface{}:
				arrayProperties[s.property] = struct{}{}
				for key, value := range v {
					labelString := fmt.Sprintf("%s=%s", key, value.(string))
					props[labelString] = struct{}{}
				}
			case []interface{}:
				arrayProperties[s.property] = struct{}{}
				for _, value := range v {
					props[value.(string)] = struct{}{}
				}
			default:
				prop = v.(string)
				props[prop] = struct{}{}
				klog.Warningf("Error formatting property with type: %+v\n", reflect.TypeOf(v))
			}

		}
		properties := stringArrayToPointer(getKeys(props))
		srchCompleteOut = append(srchCompleteOut, properties...)
	} else {
		klog.Error("searchCompleteResults rows is nil", srchCompleteOut)
	}
	if len(srchCompleteOut) > 0 {
		//Check if results are date or number
		isNumber := isNumber(srchCompleteOut)
		if isNumber { //check if valid number
			isNumberStr := "isNumber"
			//isNumber should be the first argument if the property is a number
			srchCompleteOutNum := []*string{&isNumberStr}
			// Sort the values in srchCompleteOut
			sort.Slice(srchCompleteOut, func(i, j int) bool {
				numA, _ := strconv.Atoi(*srchCompleteOut[i])
				numB, _ := strconv.Atoi(*srchCompleteOut[j])
				return numA < numB
			})
			if len(srchCompleteOut) > 1 {
				// Pass only the min and max values of the numbers to show the range in the UI
				// Temporarily disable gosec G602, which produces a false positive.
				// See https://github.com/securego/gosec/issues/1005.
				srchCompleteOut = append(srchCompleteOutNum, srchCompleteOut[0],
					srchCompleteOut[len(srchCompleteOut)-1]) // #nosec G602
			} else {
				srchCompleteOut = append(srchCompleteOutNum, srchCompleteOut...)
			}

		}
		if !isNumber && isDate(srchCompleteOut) { //check if valid date
			isDateStr := "isDate"
			srchCompleteOutDate := []*string{&isDateStr}
			srchCompleteOut = srchCompleteOutDate
		}
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
