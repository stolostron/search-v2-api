// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"fmt"
	"strings"
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
	"k8s.io/klog/v2"
)

// Constants for jsonb data extraction
const jsonbExtractOperator = "data->>?"

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

			opValueMap := matchOperatorToProperty("string", map[string][]string{},
				PointerToStringArray(filter.Values), filter.Property)
			klog.V(5).Infof("Extract operator from managedHub filter: %+v \n", opValueMap)

			for key, values := range opValueMap {
				klog.V(5).Info("Processing ", key, " values ", values, " in opValueMap for managedHub filter")
				if processOpValueMapManagedHub(key, values) {
					return true
				}
			}
			return false
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
		klog.V(5).Info("No uids selected for query:Related()")
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

// Example query: SELECT uid, cluster, data FROM search.resources  WHERE lower(data->> 'kind') IN
// (lower('Pod')) AND lower(data->> 'cluster') IN (lower('local-cluster')) LIMIT 1000
func (s *SearchResult) buildSearchQuery(ctx context.Context, count bool, uid bool) error {
	var limit uint
	var selectDs *goqu.SelectDataset
	var whereDs []exp.Expression
	var params []interface{}
	var sql string
	var err error

	// define schema table:
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)

	if len(s.input.Keywords) > 0 {
		jsb := goqu.L("jsonb_each_text(?)", goqu.C("data"))
		ds = goqu.From(schemaTable, jsb)
	}

	if s.input != nil && (len(s.input.Filters) > 0 || len(s.input.Keywords) > 0) {
		// WHERE CLAUSE
		whereDs, s.propTypes, err = WhereClauseFilter(s.context, s.input, s.propTypes)
		if err != nil {
			s.checkErrorBuildingQuery(err, ErrorMsg)
			return err
		}

		// SELECT CLAUSE
		selectDs = s.buildSelectClause(ds, count, uid)

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
			s.checkErrorBuildingQuery(fmt.Errorf("RBAC clause is required! None found for search query %+v for user %s with uid %s ",
				s.input, userInfo.Username, userInfo.UID), ErrorMsg)
			return fmt.Errorf("RBAC clause is required! None found for search query %+v for user %s with uid %s ",
				s.input, userInfo.Username, userInfo.UID)
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

	// Build query with WHERE clause
	queryDs := selectDs.Where(whereDs...)

	// ORDER BY CLAUSE
	if !count && s.input.OrderBy != nil && *s.input.OrderBy != "" {
		queryDs, err = s.applyOrderBy(queryDs)
		if err != nil {
			s.checkErrorBuildingQuery(err, ErrorMsg)
			return err
		}
	}

	// OFFSET CLAUSE
	if !count && s.input.Offset != nil {
		if *s.input.Offset < 0 {
			err := fmt.Errorf("invalid offset: %d. Offset must be non-negative", *s.input.Offset)
			s.checkErrorBuildingQuery(err, ErrorMsg)
			return err
		}
		if *s.input.Offset > 0 {
			// Safe conversion: already checked > 0, so non-negative
			offset := uint(*s.input.Offset) // #nosec G115 - Validated positive via > 0 check
			queryDs = queryDs.Offset(offset)
		}
		// If offset == 0, no need to apply OFFSET clause (equivalent to no offset)
	}

	// LIMIT CLAUSE
	if limit != 0 {
		queryDs = queryDs.Limit(limit)
	}

	// Get the query
	sql, params, err = queryDs.ToSQL()
	if err != nil {
		s.checkErrorBuildingQuery(err, ErrorMsg)
	}
	klog.V(5).Infof("Search query: %s\nargs: %s", sql, params)
	s.query = sql
	s.params = params
	return err
}

// buildSelectClause constructs the SELECT clause based on query type (count, uid, or items).
// For items queries with orderBy, includes the order field in SELECT to satisfy PostgreSQL's
// DISTINCT + ORDER BY requirement.
func (s *SearchResult) buildSelectClause(ds *goqu.SelectDataset, count bool, uid bool) *goqu.SelectDataset {
	if count {
		return ds.Select(goqu.COUNT("uid"))
	}

	if uid {
		return ds.Select("uid")
	}

	// Items query with possible ORDER BY
	if s.input.OrderBy != nil && *s.input.OrderBy != "" {
		orderProperty := s.extractOrderByProperty()
		if orderProperty != "" {
			// 'cluster' and 'uid' are already in SELECT, no need to add them again
			if orderProperty == "cluster" || orderProperty == "uid" {
				return ds.SelectDistinct("uid", "cluster", "data")
			}
			// Include the order field in the SELECT to make it compatible with DISTINCT
			return ds.SelectDistinct("uid", "cluster", "data", goqu.L(jsonbExtractOperator, orderProperty))
		}
	}

	return ds.SelectDistinct("uid", "cluster", "data")
}

// extractOrderByProperty extracts just the property name from the orderBy string.
// Returns empty string if orderBy is nil or invalid.
// Handles edge cases like leading/trailing whitespace: " name asc" -> "name"
func (s *SearchResult) extractOrderByProperty() string {
	if s.input.OrderBy == nil || *s.input.OrderBy == "" {
		return ""
	}

	orderByStr := strings.TrimSpace(*s.input.OrderBy)
	if orderByStr == "" {
		return "" // Was only whitespace
	}

	// Parse to get the property name (first part before space)
	if idx := strings.Index(orderByStr, " "); idx != -1 {
		return orderByStr[:idx]
	}
	// No space found, entire string is the property
	return orderByStr
}

// applyOrderBy parses the orderBy string and applies it to the query.
// Expected format: "property_name asc" or "property_name desc"
// Example: "name desc" or "created asc"
func (s *SearchResult) applyOrderBy(queryDs *goqu.SelectDataset) (*goqu.SelectDataset, error) {
	if s.input.OrderBy == nil || *s.input.OrderBy == "" {
		return queryDs, nil
	}

	orderByStr := *s.input.OrderBy
	klog.V(5).Infof("Applying ORDER BY: %s", orderByStr)

	// Parse the orderBy string (format: "property [asc|desc]")
	// strings.Fields splits by whitespace and removes empty parts
	parts := strings.Fields(orderByStr)

	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid orderBy format: '%s'. Expected 'property [asc|desc]'", orderByStr)
	}

	property := parts[0]
	direction := "asc" // Default direction

	// Validate direction if provided
	if len(parts) > 1 {
		dirLower := strings.ToLower(parts[1])
		if dirLower != "asc" && dirLower != "desc" {
			return nil, fmt.Errorf("invalid orderBy direction: '%s'. Expected 'asc' or 'desc'", parts[1])
		}
		direction = dirLower
	}

	// Reject extra parts beyond property and direction
	if len(parts) > 2 {
		return nil, fmt.Errorf(
			"invalid orderBy format: '%s'. Expected 'property [asc|desc]', but got %d parts",
			orderByStr, len(parts))
	}

	// Build the ORDER BY expression
	// 'cluster' and 'uid' are table columns, not in jsonb
	// All other properties are in the 'data' jsonb column
	var orderExp exp.OrderedExpression
	if property == "cluster" || property == "uid" {
		// Direct column reference
		if direction == "desc" {
			orderExp = goqu.C(property).Desc()
		} else {
			orderExp = goqu.C(property).Asc()
		}
	} else {
		// JSON field: use data->>'property'
		if direction == "desc" {
			orderExp = goqu.L(jsonbExtractOperator, property).Desc()
		} else {
			orderExp = goqu.L(jsonbExtractOperator, property).Asc()
		}
	}

	return queryDs.Order(orderExp), nil
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

		// Check if the query actually contains the order field in SELECT by looking at the query string
		// The order field is only added for SELECT DISTINCT queries (main items),
		// NOT for related items queries which use regular SELECT
		hasOrderFieldInQuery := s.input.OrderBy != nil && *s.input.OrderBy != "" &&
			strings.Contains(s.query, "data->>'")

		if hasOrderFieldInQuery {
			var orderValue interface{} // Scan the order column but don't use it
			err = rows.Scan(&uid, &cluster, &data, &orderValue)
		} else {
			err = rows.Scan(&uid, &cluster, &data)
		}

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

	if len(input.Keywords) > 0 {
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
				klog.V(3).Infof("For filter prop: %s, datatype is :%s dataTypeInMap: %t\n", filter.Property,
					dataType, dataTypeInMap)
				if err != nil {
					klog.Errorf("Error creating property type map with err: [%s]", err)
					return whereDs, propTypeMap, fmt.Errorf("error [%s] fetching data type for property: [%s]",
						err, filter.Property)
				}
				if !dataTypeInMap {
					klog.V(1).Infof("Input property type [%s] doesn't exist, setting false condition to return 0 results", filter.Property)
					// search=> explain analyze select * from search.resources where 1 = 0;
					//                                     QUERY PLAN
					//------------------------------------------------------------------------------------
					// Result  (cost=0.00..0.00 rows=0 width=0) (actual time=0.001..0.001 rows=0 loops=1)
					//   One-Time Filter: false
					// Planning Time: 0.060 ms
					// Execution Time: 0.008 ms
					// (4 rows)
					whereDs = append(whereDs, goqu.L("1 = 0").Expression())
					return whereDs, propTypeMap, err
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
