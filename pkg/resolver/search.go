// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/driftprogramming/pgxpoolmock"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stolostron/search-v2-api/graph/model"
	db "github.com/stolostron/search-v2-api/pkg/database"
	"github.com/stolostron/search-v2-api/pkg/metric"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	v1 "k8s.io/api/authentication/v1"
	"k8s.io/klog/v2"
)

type SearchResult struct {
	context   context.Context
	input     *model.SearchInput
	level     int // The number of levels/hops for finding relationships for a particular resource
	params    []interface{}
	pool      pgxpoolmock.PgxPool // Used to mock database pool in tests
	propTypes map[string]string
	query     string
	uids      []*string // List of uids from search result to be used to get relatioinships.
	userData  *rbac.UserData
	wg        sync.WaitGroup // Used to serialize search query and relatioinships query.
}

const ErrorMsg string = "Error building Search query"

func Search(ctx context.Context, input []*model.SearchInput) ([]*SearchResult, error) {
	defer metric.SlowLog("SearchResolver", 0)()
	// For each input, create a SearchResult resolver.
	srchResult := make([]*SearchResult, len(input))
	userData, userDataErr := rbac.GetCache().GetUserData(ctx)
	if userDataErr != nil {
		return srchResult, userDataErr
	}

	// check that shared cache has resource datatypes
	propTypes, err := getPropertyType(ctx, false)
	if err != nil {
		klog.Warningf("Error creating datatype map. Error: [%s] ", err)
	}

	// Proceed if user's rbac data exists
	if len(input) > 0 {
		for index, in := range input {
			srchResult[index] = &SearchResult{
				input:     in,
				pool:      db.GetConnection(),
				userData:  userData,
				context:   ctx,
				propTypes: propTypes,
			}
		}
	}
	return srchResult, nil

}

func (s *SearchResult) Count() int {
	klog.V(2).Info("Resolving SearchResult:Count()")
	t := time.Now()
	s.buildSearchQuery(s.context, true, false)
	count := s.resolveCount()
	klog.V(1).Info("Time took to build Coounts: ", time.Since(t))

	return count
}

func (s *SearchResult) Items() []map[string]interface{} {
	s.wg.Add(1)
	defer s.wg.Done()
	klog.V(2).Info("Resolving SearchResult:Items()")
	s.buildSearchQuery(s.context, false, false)

	r, e := s.resolveItems()
	if e != nil {
		klog.Error("Error resolving items.", e)
	}
	return r
}

func (s *SearchResult) Related(ctx context.Context) []SearchRelatedResult {
	klog.V(2).Info("Resolving SearchResult:Related()")
	if s.context == nil {
		s.context = ctx
	}
	if s.uids == nil {
		s.Uids()
	}
	var start time.Time
	var numUIDs int
	var timer *prometheus.Timer
	s.wg.Wait()
	var r []SearchRelatedResult

	if len(s.uids) > 0 {
		start = time.Now()
		//create metric and set labels
		HttpDurationByQuery := metric.HttpDurationByLabels(prometheus.Labels{"action": "related_query"})

		//create timer and return observed duration
		timer = prometheus.NewTimer(HttpDurationByQuery.WithLabelValues("200")) //change labels
		defer timer.ObserveDuration()

		numUIDs = len(s.uids)
		r = s.getRelationResolvers(ctx)
	} else {
		klog.Warning("No uids selected for query:Related()")
	}
	defer func() {
		if len(s.uids) > 0 { // Log a warning if finding relationships is too slow.
			// Note the 500ms is just an initial guess, we should adjust based on normal execution time.
			if time.Since(start) > 500*time.Millisecond {
				klog.Warningf("Finding relationships for %d uids and %d level(s) took %s.",
					numUIDs, s.level, time.Since(start))
				return
			}
			klog.V(1).Infof("Finding relationships for %d uids and %d level(s) took %s.",
				numUIDs, s.level, time.Since(start))
		} else {
			klog.V(4).Infof("Not finding relationships as there are %d uids and %d level(s).",
				len(s.uids), s.level)
		}
	}()
	return r
}

func (s *SearchResult) Uids() {
	klog.V(2).Info("Resolving SearchResult:Uids()")
	s.buildSearchQuery(s.context, false, true)
	s.resolveUids()
}

// Build where clause with rbac by combining clusterscoped, namespace scoped and managed cluster access
func buildRbacWhereClause(ctx context.Context, userrbac *rbac.UserData, userInfo v1.UserInfo) exp.ExpressionList {

	return goqu.Or(
		matchManagedCluster(getKeys(userrbac.ManagedClusters)), // goqu.I("cluster").In([]string{"clusterNames", ....})
		matchHubCluster(userrbac, userInfo),
	)

}

// Example query: SELECT uid, cluster, data FROM search.resources  WHERE lower(data->> 'kind') IN
// (lower('Pod')) AND lower(data->> 'cluster') IN (lower('local-cluster')) LIMIT 1000
func (s *SearchResult) buildSearchQuery(ctx context.Context, count bool, uid bool) {
	var limit int
	var selectDs *goqu.SelectDataset
	var whereDs []exp.Expression
	var params []interface{}
	var sql string
	var err error

	//create metric and set labels
	HttpDurationByQuery := metric.HttpDurationByLabels(prometheus.Labels{"action": "build_search_query"})

	//create timer and return observed duration
	timer := prometheus.NewTimer(HttpDurationByQuery.WithLabelValues("200")) //change labels
	defer timer.ObserveDuration()

	// define schema table:
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)

	if s.input.Keywords != nil && len(s.input.Keywords) > 0 {
		jsb := goqu.L("jsonb_each_text(?)", goqu.C("data"))
		ds = goqu.From(schemaTable, jsb)
	}

	if s.input != nil && (len(s.input.Filters) > 0 || (s.input.Keywords != nil && len(s.input.Keywords) > 0)) {
		// WHERE CLAUSE
		whereDs, s.propTypes, err = WhereClauseFilter(s.context, s.input, s.propTypes)
		if err != nil {
			s.checkErrorBuildingQuery(err, ErrorMsg)
			return
		}

		// SELECT CLAUSE
		if count {
			selectDs = ds.Select(goqu.COUNT("uid"))
		} else if uid {
			selectDs = ds.Select("uid")
		} else {
			selectDs = ds.SelectDistinct("uid", "cluster", "data")
		}

		sql, _, err = selectDs.Where(whereDs...).ToSQL() // use original query
		klog.V(3).Info("Search query before adding RBAC clause:", sql, " error:", err)

		_, userInfo := rbac.GetCache().GetUserUID(ctx)
		// RBAC CLAUSE
		if s.userData != nil {
			//create metric and set labels
			HttpDurationByQuery := metric.HttpDurationByLabels(prometheus.Labels{"action": "build_rbac_query"})

			//create timer and return observed duration
			timer = prometheus.NewTimer(HttpDurationByQuery.WithLabelValues("200")) //change labels
			defer timer.ObserveDuration()
			whereDs = append(whereDs,
				buildRbacWhereClause(ctx, s.userData, userInfo)) // add rbac
			if len(whereDs) == 0 {
				s.checkErrorBuildingQuery(fmt.Errorf("search query must contain a whereClause"),
					ErrorMsg)
				return
			}
		} else {
			errorStr := fmt.Sprintf("RBAC clause is required! None found for search query %+v for user %s with uid %s ",
				s.input, userInfo.Username, userInfo.UID)
			s.checkErrorBuildingQuery(fmt.Errorf(errorStr), ErrorMsg)
			return
		}
	} else {
		s.checkErrorBuildingQuery(fmt.Errorf("query input must contain a filter or keyword. Received: %+v",
			s.input), ErrorMsg)
		return
	}

	// LIMIT CLAUSE
	if !count {
		limit = s.setLimit()
	}

	// Get the query
	if limit != 0 {
		sql, params, err = selectDs.Where(whereDs...).Limit(uint(limit)).ToSQL()
	} else {
		sql, params, err = selectDs.Where(whereDs...).ToSQL()
	}
	if err != nil {
		klog.Errorf("Error building Search query: %s", err.Error())
	}

	klog.V(5).Infof("Search query: %s\nargs: %s", sql, params)
	s.query = sql
	s.params = params
}

func (s *SearchResult) checkErrorBuildingQuery(err error, logMessage string) {
	klog.Error(logMessage, " ", err)

	s.query = ""
	s.params = nil
}

func (s *SearchResult) resolveCount() int {
	rows := s.pool.QueryRow(context.TODO(), s.query, s.params...)
	//create metric and set labels
	HttpDurationByQuery := metric.HttpDurationByLabels(prometheus.Labels{"action": "count_query"})

	//create timer and return observed duration
	timer := prometheus.NewTimer(HttpDurationByQuery.WithLabelValues("200")) //change labels
	defer timer.ObserveDuration()
	var count int
	err := rows.Scan(&count)
	if err != nil {
		klog.Errorf("Error %s resolving count for query:%s", err.Error(), s.query)
	}
	return count
}

func (s *SearchResult) resolveUids() {
	rows, err := s.pool.Query(s.context, s.query, s.params...)
	if err != nil {
		klog.Errorf("Error resolving query [%s] with args [%+v]. Error: [%+v]", s.query, s.params, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var uid string
		err = rows.Scan(&uid)
		if err != nil {
			klog.Errorf("Error %s retrieving rows for query:%s", err.Error(), s.query)
		}
		s.uids = append(s.uids, &uid)
	}

}
func (s *SearchResult) resolveItems() ([]map[string]interface{}, error) {
	items := []map[string]interface{}{}

	//create metric and set labels
	HttpDurationByQuery := metric.HttpDurationByLabels(prometheus.Labels{"action": "items_query"})

	//create timer and return observed duration
	timer := prometheus.NewTimer(HttpDurationByQuery.WithLabelValues("200")) //change labels
	defer timer.ObserveDuration()

	klog.V(5).Infof("Query issued by resolver [%s] ", s.query)
	rows, err := s.pool.Query(s.context, s.query, s.params...)

	if err != nil {
		klog.Errorf("Error resolving query [%s] with args [%+v]. Error: [%+v]", s.query, s.params, err)
		return items, err
	}
	defer rows.Close()

	s.uids = make([]*string, len(items))

	for rows.Next() {
		var uid string
		var cluster string
		var data map[string]interface{}
		err = rows.Scan(&uid, &cluster, &data)
		if err != nil {
			klog.Errorf("Error %s retrieving rows for query:%s", err.Error(), s.query)
		}
		currItem := formatDataMap(data)
		currItem["_uid"] = uid
		currItem["cluster"] = cluster

		items = append(items, currItem)
		s.uids = append(s.uids, &uid)

	}

	return items, nil
}

func WhereClauseFilter(ctx context.Context, input *model.SearchInput,
	propTypeMap map[string]string) ([]exp.Expression, map[string]string, error) {
	var opDateValueMap map[string][]string

	var whereDs []exp.Expression
	var dataTypeFromMap string
	var err error

	if input.Keywords != nil && len(input.Keywords) > 0 {
		// Sample query: SELECT COUNT("uid") FROM "search"."resources", jsonb_each_text("data")
		// WHERE (("value" LIKE '%dns%') AND ("data"->>'kind' ILIKE ANY ('{"pod","deployment"}')))
		keywords := pointerToStringArray(input.Keywords)
		for _, key := range keywords {
			key = "%" + key + "%"
			whereDs = append(whereDs, goqu.L(`"value"`).ILike(key).Expression())
		}
	}

	if input.Filters != nil {
		for _, filter := range input.Filters {
			if len(filter.Values) == 0 {
				klog.Warningf("Ignoring filter [%s] because it has no values", filter.Property)
				continue
			}
			values := pointerToStringArray(filter.Values)

			dataType, dataTypeInMap := propTypeMap[filter.Property]
			if len(propTypeMap) == 0 || !dataTypeInMap {
				klog.V(3).Info("Property type for [%s] doesn't exist in cache. Refreshing property type cache",
					filter.Property)
				propTypeMapNew, err := getPropertyType(ctx, true) // Refresh the property type cache.
				if err != nil {
					klog.Errorf("Error creating property type map with err: [%s] ", err)
					break
				}

				propTypeMap = propTypeMapNew

				dataType, dataTypeInMap = propTypeMap[filter.Property]
				klog.Infof("For filter prop: %s, datatype is :%s dataTypeInMap: %t\n", filter.Property,
					dataType, dataTypeInMap)
			}

			if len(propTypeMap) > 0 && dataTypeInMap {
				klog.V(5).Infof("For filter prop: %s, datatype is :%s\n", filter.Property, dataType)

				// if property matches then call decode function:
				values, dataTypeFromMap = decodePropertyTypes(values, dataType)
				// Check if value is a number or date and get the cleaned up value
				opDateValueMap = getOperatorAndNumDateFilter(filter.Property, values, dataTypeFromMap)
			} else {
				klog.Error("Error with property type list is empty.")
				values = decodePropertyTypesNoPropMap(values, filter)
				// Check if value is a number or date and get the cleaned up value
				opDateValueMap = getOperatorAndNumDateFilter(filter.Property, values, nil)

			}
			//Sort map according to keys - This is for the ease/stability of tests when there are multiple operators
			keys := getKeys(opDateValueMap)
			var operatorWhereDs []exp.Expression //store all the clauses for this filter together
			for _, operator := range keys {
				operatorWhereDs = append(operatorWhereDs,
					getWhereClauseExpression(filter.Property, operator, opDateValueMap[operator])...)
			}

			whereDs = append(whereDs, goqu.Or(operatorWhereDs...)) //Join all the clauses with OR

		}
	}

	return whereDs, propTypeMap, err
}
