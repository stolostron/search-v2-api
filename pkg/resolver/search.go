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
	input     *model.SearchInput
	pool      pgxpoolmock.PgxPool
	uids      []*string      // List of uids from search result to be used to get relatioinships.
	wg        sync.WaitGroup // WORKAROUND: Used to serialize search query and relatioinships query.
	query     string
	params    []interface{}
	level     int // The number of levels/hops for finding relationships for a particular resource
	propTypes map[string]string
	userData  *rbac.UserData
	context   context.Context
}

func Search(ctx context.Context, input []*model.SearchInput) ([]*SearchResult, error) {
	// For each input, create a SearchResult resolver.
	srchResult := make([]*SearchResult, len(input))
	userData, userDataErr := rbac.GetCache().GetUserData(ctx)
	if userDataErr != nil {
		return srchResult, userDataErr
	}

	//check that shared cache has resource datatypes:
	propTypesCache, err := GetPropertyTypeCache(ctx)
	if err != nil {
		klog.Warningf("Error creating datatype map with err: [%s] ", err)
	}

	// Proceed if user's rbac data exists
	if len(input) > 0 {
		for index, in := range input {
			srchResult[index] = &SearchResult{
				input:     in,
				pool:      db.GetConnection(),
				userData:  userData,
				context:   ctx,
				propTypes: propTypesCache,
			}
		}
	}
	return srchResult, nil

}

func (s *SearchResult) Count() int {
	klog.V(2).Info("Resolving SearchResult:Count()")
	s.buildSearchQuery(s.context, true, false)
	count := s.resolveCount()

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
	if s.uids == nil {
		s.Uids()
	}
	if s.context == nil {
		s.context = ctx
	}
	var start time.Time
	var numUIDs int

	s.wg.Wait()
	var r []SearchRelatedResult

	if len(s.uids) > 0 {
		start = time.Now()
		numUIDs = len(s.uids)
		r = s.getRelations(ctx)
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
			klog.V(4).Infof("Finding relationships for %d uids and %d level(s) took %s.",
				numUIDs, s.level, time.Since(start))
		} else {
			klog.V(4).Infof("Not finding relationships as there are %d uids and %d level(s).",
				numUIDs, s.level)
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
		matchManagedCluster(GetKeys(userrbac.ManagedClusters)), // goqu.I("cluster").In([]string{"clusterNames", ....})
		goqu.And(
			matchHubCluster(), // goqu.L(`data->>?`, "_hubClusterResource").Eq("true")
			goqu.Or(
				matchClusterScopedResources(userrbac.CsResources, userInfo), // (namespace=null AND apigroup AND kind)
				matchNamespacedResources(userrbac.NsResources, userInfo),    // (namespace AND apiproup AND kind)
			),
		),
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

	//define schema table:
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)

	if s.input.Keywords != nil && len(s.input.Keywords) > 0 {
		jsb := goqu.L("jsonb_each_text(?)", goqu.C("data"))
		ds = goqu.From(schemaTable, jsb)
	}

	if s.input != nil && (len(s.input.Filters) > 0 || (s.input.Keywords != nil && len(s.input.Keywords) > 0)) {
		//WHERE CLAUSE
		whereDs, s.propTypes, err = WhereClauseFilter(s.context, s.input, s.propTypes)
		if whereDs != nil {

			//SELECT CLAUSE
			if count {
				selectDs = ds.Select(goqu.COUNT("uid"))
			} else if uid {
				selectDs = ds.Select("uid")
			} else {
				selectDs = ds.SelectDistinct("uid", "cluster", "data")
			}

			sql, _, err = selectDs.Where(whereDs...).ToSQL() //use original query
			klog.V(3).Info("Search query before adding RBAC clause:", sql, " error:", err)

			//RBAC CLAUSE
			if s.userData != nil {
				_, userInfo := rbac.GetCache().GetUserUID(ctx)
				whereDs = append(whereDs,
					buildRbacWhereClause(ctx, s.userData, userInfo)) // add rbac
			} else {
				panic(fmt.Sprintf("RBAC clause is required! None found for search query %+v for user %s ", s.input,
					ctx.Value(rbac.ContextAuthTokenKey)))
			}
		} else if err != nil {
			klog.Errorf("Error building Search query: %s", err.Error())
		}
	}

	//LIMIT CLAUSE
	if !count {
		limit = s.SetLimit()
	}

	//Get the query
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

func (s *SearchResult) resolveCount() int {
	rows := s.pool.QueryRow(context.TODO(), s.query, s.params...)

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
	timer := prometheus.NewTimer(metric.DBQueryDuration.WithLabelValues("resolveItemsFunc"))
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
		currItem := FormatDataMap(data)
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
	var dataTypeFromMap string
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
			if len(filter.Values) > 0 {
				values := PointerToStringArray(filter.Values)

				if len(propTypeMap) > 0 {
					if dataTypeInMap, ok := propTypeMap[filter.Property]; !ok { //check if value exists/if value doesn't exist
						klog.Warningf("Property type for [%s] doesn't exist in cache. Refreshing property type cache",
							filter.Property)
						propTypeMapNew, err := GetPropertyTypeCache(ctx) //call database cache again
						if err != nil {
							klog.Warningf("Error creating property type map with err: [%s] ", err)
						}
						propTypeMap = propTypeMapNew
						break

					} else {

						klog.V(5).Infof("Prop in map:%s, filter prop is: %s, datatype :%s\n", dataTypeInMap, filter.Property)
						//if property mactches then call decode function:
						values, dataTypeFromMap = DecodePropertyTypes(values, dataTypeInMap)

						// Check if value is a number or date and get the cleaned up value
						opDateValueMap := GetOperatorAndNumDateFilter(filter.Property, values, dataTypeFromMap)
						//Sort map according to keys - This is for the ease/stability of tests when there are multiple operators
						keys := GetKeys(opDateValueMap)
						var operatorWhereDs []exp.Expression //store all the clauses for this filter together
						for _, operator := range keys {
							operatorWhereDs = append(operatorWhereDs,
								GetWhereClauseExpression(filter.Property, operator, opDateValueMap[operator], dataTypeFromMap)...)
						}

						whereDs = append(whereDs, goqu.Or(operatorWhereDs...)) //Join all the clauses with OR
						fmt.Println(whereDs)
					}
				} else {
					//if map is empty don't return anything
					klog.Error("Error with property type list is empty.")
					return nil, nil, err
				}
			} else {
				klog.Warningf("Ignoring filter [%s] because it has no values", filter.Property)

			}
		}
	}

	return whereDs, propTypeMap, err
}
