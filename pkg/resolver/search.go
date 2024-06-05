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
	"github.com/stolostron/search-v2-api/pkg/config"
	db "github.com/stolostron/search-v2-api/pkg/database"
	"github.com/stolostron/search-v2-api/pkg/metrics"
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
	userData  rbac.UserData
	wg        sync.WaitGroup // Used to serialize search query and relatioinships query.
}

const ErrorMsg string = "Error building Search query:"

func Search(ctx context.Context, input []*model.SearchInput) ([]*SearchResult, error) {
	defer metrics.SlowLog("SearchResolver", 0)()
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
				pool:      db.GetConnPool(ctx),
				userData:  userData,
				context:   ctx,
				propTypes: propTypes,
			}
		}
	}
	return srchResult, nil

}

// Stop search if managedHub is a filter and current hub name is not in values.
// Otherwise, proceed with the search.
func (s *SearchResult) matchesManagedHubFilter() bool {
	klog.V(7).Info("HUB_NAME is ", config.Cfg.HubName)
	for _, filter := range s.input.Filters {
		if filter.Property == "managedHub" {
			klog.V(5).Infof("managedHub filter: %s values: %+v \n", filter.Property,
				PointerToStringArray(filter.Values))

			opValueMap := matchOperatorToProperty("string", map[string][]string{}, PointerToStringArray(filter.Values), filter.Property)
			klog.V(5).Infof("Extract operator from managedHub filter: %+v \n", opValueMap)
			proceedWithSearch := make([]bool, len(opValueMap))
			count := 0
			for key, values := range opValueMap {
				klog.Info("Processing ", key, " values ", values, " in opValueMap for managedHub filter")
				proceedWithSearch[count] = processOpValueMapManagedHub(key, values)
				count++
			}
			return anyTrue(proceedWithSearch) // check if any of the patterns match
		}
	}
	klog.V(4).Infof("managedHub filter not present. Proceeding with Search")
	return true
}

func (s *SearchResult) Count() (int, error) {
	if !s.matchesManagedHubFilter() { // if current hub is not part of managedHub filter, stop search
		return 0, nil
	}
	klog.V(2).Info("Resolving SearchResult:Count()")
	err := s.buildSearchQuery(s.context, true, false)
	if err != nil {
		return 0, err
	}
	return s.resolveCount()
}

func (s *SearchResult) Items() ([]map[string]interface{}, error) {
	s.wg.Add(1)
	defer s.wg.Done()
	if !s.matchesManagedHubFilter() { // if current hub is not part of managedHub filter, stop search
		return []map[string]interface{}{}, nil
	}
	klog.V(2).Info("Resolving SearchResult:Items()")
	err := s.buildSearchQuery(s.context, false, false)
	if err != nil {
		return nil, err
	}
	r, e := s.resolveItems()
	if e != nil {
		s.checkErrorBuildingQuery(e, "Error resolving items.")
	}
	return r, e
}

func (s *SearchResult) Related(ctx context.Context) ([]SearchRelatedResult, error) {
	var r []SearchRelatedResult
	if !s.matchesManagedHubFilter() { // if current hub is not part of managedHub filter, stop search
		return r, nil
	}
	if s.context == nil {
		s.context = ctx
	}
	if s.uids == nil {
		err := s.Uids()
		if err != nil {
			return r, err
		}
	}
	// Wait for search to complete before resolving relationships.
	s.wg.Wait()
	// Log if this function is slow.
	defer metrics.SlowLog(fmt.Sprintf("SearchResult::Related() - uids: %d levels: %d", len(s.uids), s.level),
		500*time.Millisecond)()

	if len(s.uids) > 0 {
		r = s.getRelationResolvers(ctx)
	} else {
		klog.V(1).Info("No uids selected for query:Related()")
	}

	return r, nil
}

func (s *SearchResult) Uids() error {
	klog.V(2).Info("Resolving SearchResult:Uids()")
	err := s.buildSearchQuery(s.context, false, true)
	if err != nil {
		return err
	}
	return s.resolveUids()
}

// Build where clause with rbac by combining clusterscoped, namespace scoped and managed cluster access
func buildRbacWhereClause(ctx context.Context, userrbac rbac.UserData, userInfo v1.UserInfo) exp.ExpressionList {
	return goqu.Or(
		matchManagedCluster(getKeys(userrbac.ManagedClusters)), // goqu.I("cluster").In([]string{"clusterNames", ....})
		matchHubCluster(userrbac, userInfo),
	)
}

// Example query: SELECT uid, cluster, data FROM search.resources  WHERE lower(data->> 'kind') IN
// (lower('Pod')) AND lower(data->> 'cluster') IN (lower('local-cluster')) LIMIT 1000
func (s *SearchResult) buildSearchQuery(ctx context.Context, count bool, uid bool) error {
	var limit int
	var selectDs *goqu.SelectDataset
	var whereDs []exp.Expression
	var params []interface{}
	var sql string
	var err error

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
			return err
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

		// if one of them is not nil, userData is not empty
		if s.userData.CsResources != nil || s.userData.NsResources != nil || s.userData.ManagedClusters != nil {
			whereDs = append(whereDs,
				buildRbacWhereClause(ctx, s.userData, userInfo)) // add rbac
			if len(whereDs) == 0 {
				s.checkErrorBuildingQuery(fmt.Errorf("search query must contain a whereClause"),
					ErrorMsg)
				return err
			}
		} else {
			errorStr := fmt.Sprintf("RBAC clause is required! None found for search query %+v for user %s with uid %s ",
				s.input, userInfo.Username, userInfo.UID)
			s.checkErrorBuildingQuery(fmt.Errorf(errorStr), ErrorMsg)
			return fmt.Errorf(errorStr)
		}
	} else {
		s.checkErrorBuildingQuery(fmt.Errorf("query input must contain a filter or keyword. Received: %+v",
			s.input), ErrorMsg)
		return fmt.Errorf("query input must contain a filter or keyword. Received: %+v",
			s.input)
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
		s.checkErrorBuildingQuery(err, ErrorMsg)
	}
	klog.V(5).Infof("Search query: %s\nargs: %s", sql, params)
	s.query = sql
	s.params = params
	return err
}

func (s *SearchResult) checkErrorBuildingQuery(err error, logMessage string) {
	klog.Error(logMessage, " ", err)

	s.query = ""
	s.params = nil
}

func (s *SearchResult) resolveCount() (int, error) {
	rows := s.pool.QueryRow(context.TODO(), s.query, s.params...)

	var count int
	err := rows.Scan(&count)
	if err != nil {
		klog.Errorf("Error resolving count. Error: %s  Query: %s", err.Error(), s.query)
	}
	return count, err
}

func (s *SearchResult) resolveUids() error {
	rows, err := s.pool.Query(s.context, s.query, s.params...)
	if err != nil {
		klog.Errorf("Error resolving UIDs. Query [%s] with args [%+v]. Error: [%+v]", s.query, s.params, err)
		return err
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
	return nil
}
func (s *SearchResult) resolveItems() ([]map[string]interface{}, error) {
	items := []map[string]interface{}{}
	timer := prometheus.NewTimer(metrics.DBQueryDuration.WithLabelValues("resolveItemsFunc"))
	klog.V(5).Infof("Query issued by resolver [%s] ", s.query)
	rows, err := s.pool.Query(s.context, s.query, s.params...)

	defer timer.ObserveDuration()
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

	var whereDs []exp.Expression
	var err error

	if input.Keywords != nil && len(input.Keywords) > 0 {
		// Sample query: SELECT COUNT("uid") FROM "search"."resources", jsonb_each_text("data")
		// WHERE (("value" LIKE '%dns%') AND ("data"->>'kind' ILIKE ANY ('{"pod","deployment"}')))
		keywords := PointerToStringArray(input.Keywords)
		for _, key := range keywords {
			key = "%" + key + "%"
			whereDs = append(whereDs, goqu.L(`"value"`).ILike(key).Expression())
		}
	}

	if input.Filters != nil {
		for _, filter := range input.Filters {
			opValueMap := map[string][]string{}
			if len(filter.Values) == 0 {
				klog.Warningf("Ignoring filter [%s] because it has no values", filter.Property)
				continue
			}
			values := PointerToStringArray(filter.Values)

			dataType, dataTypeInMap := propTypeMap[filter.Property]
			if len(propTypeMap) == 0 || !dataTypeInMap {
				klog.V(3).Infof("Property type for [%s] doesn't exist in cache. Refreshing property type cache",
					filter.Property)
				propTypeMapNew, err := getPropertyType(ctx, true) // Refresh the property type cache.
				propTypeMap = propTypeMapNew
				dataType, dataTypeInMap = propTypeMap[filter.Property]
				klog.Infof("For filter prop: %s, datatype is :%s dataTypeInMap: %t\n", filter.Property,
					dataType, dataTypeInMap)
				if err != nil || !dataTypeInMap {
					klog.Errorf("Error creating property type map with err: [%s] or datatype for  [%s] not found in map",
						err, filter.Property)
					return whereDs, propTypeMap, fmt.Errorf("error [%s] fetching data type for property: [%s]",
						err, filter.Property)
				}
			}

			klog.V(5).Infof("For filter prop: %s, datatype is :%s\n", filter.Property, dataType)

			// if property matches then call decode function:
			values, err = decodePropertyTypes(values, dataType)
			if err != nil {
				return whereDs, propTypeMap, err
			}
			opValueMap = matchOperatorToProperty(dataType, opValueMap, values, filter.Property)

			//Sort map according to keys - This is for the ease/stability of tests when there are multiple operators
			keys := getKeys(opValueMap)
			var operatorWhereDs []exp.Expression //store all the clauses for this filter together
			for _, operator := range keys {
				operatorWhereDs = append(operatorWhereDs,
					getWhereClauseExpression(filter.Property, operator, opValueMap[operator], propTypeMap[filter.Property])...)
			}
			whereDs = append(whereDs, goqu.Or(operatorWhereDs...)) //Join all the clauses with OR
		}
	}

	return whereDs, propTypeMap, err
}
