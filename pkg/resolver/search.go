// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/driftprogramming/pgxpoolmock"
	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
	db "github.com/stolostron/search-v2-api/pkg/database"
	"github.com/stolostron/search-v2-api/pkg/metric"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	"k8s.io/klog/v2"
)

type SearchResult struct {
	input  *model.SearchInput
	pool   pgxpoolmock.PgxPool
	uids   []*string      // List of uids from search result to be used to get relatioinships.
	wg     sync.WaitGroup // WORKAROUND: Used to serialize search query and relatioinships query.
	query  string
	params []interface{}
	level  int // The number of levels/hops for finding relationships for a particular resource
	//  Related []SearchRelatedResult
	userData *rbac.UserData
	context  context.Context
}

func Search(ctx context.Context, input []*model.SearchInput) ([]*SearchResult, error) {
	// For each input, create a SearchResult resolver.
	srchResult := make([]*SearchResult, len(input))
	userAccess, userDataErr := getUserDataCache(ctx)
	if userDataErr != nil {
		return srchResult, userDataErr
	}
	// Proceed if user's rbac data exists
	if len(input) > 0 {
		for index, in := range input {
			srchResult[index] = &SearchResult{
				input:    in,
				pool:     db.GetConnection(),
				userData: userAccess,
				context:  ctx,
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

func (s *SearchResult) Related() []SearchRelatedResult {
	klog.V(2).Info("Resolving SearchResult:Related()")
	if s.uids == nil {
		s.Uids()
	}
	var start time.Time
	var numUIDs int

	s.wg.Wait()
	var r []SearchRelatedResult

	if len(s.uids) > 0 {
		start = time.Now()
		numUIDs = len(s.uids)
		r = s.getRelations()
	} else {
		klog.Warning("No uids selected for query:Related()")
	}
	defer func() {
		// Log a warning if finding relationships is too slow.
		// Note the 500ms is just an initial guess, we should adjust based on normal execution time.
		if time.Since(start) > 500*time.Millisecond {
			klog.Warningf("Finding relationships for %d uids and %d level(s) took %s.",
				numUIDs, s.level, time.Since(start))
			return
		}
		klog.V(4).Infof("Finding relationships for %d uids and %d level(s) took %s.",
			numUIDs, s.level, time.Since(start))
	}()
	return r
}

func (s *SearchResult) Uids() {
	klog.V(2).Info("Resolving SearchResult:Uids()")
	s.buildSearchQuery(s.context, false, true)
	s.resolveUids()
}

func Iskubeadmin(ctx context.Context) bool {
	_, userDetails := rbac.CacheInst.GetUserUID(ctx)
	if userDetails.Username == "kube:admin" {
		klog.Warning("TEMPORARY WORKAROUND for Kubeadmin: Turning off RBAC")
		return true
	}
	for _, group := range userDetails.Groups {
		if group == "system:cluster-admins" {
			klog.Warning("TEMPORARY WORKAROUND for Kubeadmin: Turning off RBAC")
			return true
		}
	}
	return false
}

// Build where clause with rbac by combining clusterscoped, namespace scoped and managed cluster access
func buildRbacWhereClause(ctx context.Context, userrbac *rbac.UserData) exp.ExpressionList {
	return goqu.Or(
		matchManagedCluster(userrbac.ManagedClusters), // goqu.I("cluster").In([]string{"clusterNames", ....})
		goqu.And(
			matchHubCluster(), // goqu.L(`data->>?`, "_hubClusterResource").Eq("true")
			goqu.Or(
				matchClusterScopedResources(userrbac.CsResources), // (namespace=null AND apigroup AND kind)
				matchNamespacedResources(userrbac.NsResources),    // (namespace AND apiproup AND kind)
			),
		),
	)
}

func (s *SearchResult) buildSearchQuery(ctx context.Context, count bool, uid bool) {
	var limit int
	var selectDs *goqu.SelectDataset
	var whereDs []exp.Expression

	// Example query: SELECT uid, cluster, data FROM search.resources  WHERE lower(data->> 'kind') IN
	// (lower('Pod')) AND lower(data->> 'cluster') IN (lower('local-cluster')) LIMIT 1000

	//define schema table:
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)

	if s.input.Keywords != nil && len(s.input.Keywords) > 0 {
		jsb := goqu.L("jsonb_each_text(?)", goqu.C("data"))
		ds = goqu.From(schemaTable, jsb)
	}

	//SELECT CLAUSE
	if count {
		selectDs = ds.Select(goqu.COUNT("uid"))
	} else if uid {
		selectDs = ds.Select("uid")
	} else {
		selectDs = ds.SelectDistinct("uid", "cluster", "data")
	}

	//WHERE CLAUSE
	if s.input != nil && (len(s.input.Filters) > 0 || (s.input.Keywords != nil && len(s.input.Keywords) > 0)) {
		whereDs = WhereClauseFilter(s.input)
		//RBAC CLAUSE
		if s.userData != nil && !Iskubeadmin(ctx) {
			whereDs = append(whereDs,
				buildRbacWhereClause(ctx, s.userData)) // add rbac

		} else {
			if !Iskubeadmin(ctx) {
				panic(fmt.Sprintf("RBAC clause is required! None found for search query %+v for user %s ", s.input,
					ctx.Value(rbac.ContextAuthTokenKey)))
			}
		}
	}

	//LIMIT CLAUSE
	if !count {
		limit = s.setLimit()
	}
	var params []interface{}
	var sql string
	var err error
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
	klog.V(5).Info("Query issued by resolver [%s] ", s.query)
	rows, err := s.pool.Query(s.context, s.query, s.params...)
	defer timer.ObserveDuration()
	if err != nil {
		klog.Errorf("Error resolving query [%s] with args [%+v]. Error: [%+v]", s.query, s.params, err)
		return items, err
	}
	defer rows.Close()

	var cluster string
	var data map[string]interface{}
	s.uids = make([]*string, len(items))

	for rows.Next() {
		var uid string
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

// Remove operator (<=, >=, !=, !, <, >, =) if any from values
func getOperator(values []string) map[string][]string {
	// Get the operator (/^<=|^>=|^!=|^!|^<|^>|^=/)
	var operator string
	// Replace any of these symbols with ""
	replacer := strings.NewReplacer("<=", "",
		">=", "",
		"!=", "",
		"!", "",
		"<", "",
		">", "",
		"=", "")
	operatorValue := map[string][]string{}

	for _, value := range values {
		operatorRemovedValue := replacer.Replace(value)
		operator = strings.Replace(value, operatorRemovedValue, "", 1) // find operator
		if vals, ok := operatorValue[operator]; !ok {
			if operator != "" { // Add to map only if operator is present
				operatorValue[operator] = []string{operatorRemovedValue} // Add an entry to map with key as operator
			}
		} else {
			vals = append(vals, operatorRemovedValue)
			operatorValue[operator] = vals
		}
	}
	return operatorValue
}

func getWhereClauseExpression(prop, operator string, values []string) []exp.Expression {
	exps := []exp.Expression{}
	jsonLookup := func(col exp.Expression, field string) goqu.Expression {
		return goqu.L("?@>?", col, field)
		// WHERE ("data"->'label' @> '{"component":"network"}')
	}
	jsonAnyKeyArrayLookup := func(col exp.Expression, fields []string) goqu.Expression {
		return goqu.L("???", col, goqu.Literal("?|"), fields)
	}

	switch operator {
	case "<=":
		for _, val := range values {
			exps = append(exps, goqu.L(`"data"->>?`, prop).Lte(val))
		}
	case ">=":
		for _, val := range values {
			exps = append(exps, goqu.L(`"data"->>?`, prop).Gte(val))
		}
	case "!=":
		exps = append(exps, goqu.L(`"data"->>?`, prop).Neq(values))

	case "!":
		exps = append(exps, goqu.L(`"data"->>?`, prop).NotIn(values))
	case "<":
		for _, val := range values {
			exps = append(exps, goqu.L(`"data"->>?`, prop).Lt(val))
		}
	case ">":
		for _, val := range values {
			exps = append(exps, goqu.L(`"data"->>?`, prop).Gt(val))
		}
	case "=":
		exps = append(exps, goqu.L(`"data"->>?`, prop).In(values))
	case "@>":
		for _, val := range values {
			exps = append(exps, jsonLookup(goqu.L(`"data"->?`, prop), val))
		}
	case "?|":
		exps = append(exps, jsonAnyKeyArrayLookup(goqu.L(`"data"->?`, prop), values))

	default:
		if prop == "cluster" {
			exps = append(exps, goqu.C(prop).In(values))
		} else if prop == "kind" { //ILIKE to enable case-insensitive comparison for kind. Needed for V1 compatibility.
			if isLower(values) {
				exps = append(exps, goqu.L(`"data"->>?`, prop).ILike(goqu.Any(pq.Array(values))))
				klog.Warning("Using ILIKE for lower case KIND string comparison.",
					"- This behavior is needed for V1 compatibility and will be deprecated with Search V2.")
			} else {
				exps = append(exps, goqu.L(`"data"->>?`, prop).In(values))
			}
		} else {
			exps = append(exps, goqu.L(`"data"->>?`, prop).In(values))
		}
	}
	return exps

}

//if any string values starts with lower case letters, return true
func isLower(values []string) bool {
	for _, str := range values {
		firstChar := rune(str[0]) //check if first character of the string is lower case
		if unicode.IsLower(firstChar) && unicode.IsLetter(firstChar) {
			return true
		}
	}
	return false
}

// Check if value is a number or date and get the operator
// Returns a map that stores operator and values
func getOperatorAndNumDateFilter(filter string, values []string) map[string][]string {

	opValueMap := getOperator(values) //If values are numbers
	// Store the operator and value in a map - this is to handle multiple values
	updateOpValueMap := func(operator string, operatorValueMap map[string][]string, operatorRemovedValue string) {
		if vals, ok := operatorValueMap[operator]; !ok {
			operatorValueMap[operator] = []string{operatorRemovedValue}
		} else {
			vals = append(vals, operatorRemovedValue)
			operatorValueMap[operator] = vals
		}
	}
	if len(opValueMap) < 1 { //If not a number (no operator), check if values are dates
		// Expected values: {"hour", "day", "week", "month", "year"}
		operator := ">" // For dates, always check for values '>'
		now := time.Now()
		for _, val := range values {
			var then string
			format := "2006-01-02T15:04:05Z"
			switch val {
			case "hour":
				then = now.Add(time.Duration(-1) * time.Hour).Format(format)

			case "day":
				then = now.AddDate(0, 0, -1).Format(format)

			case "week":
				then = now.AddDate(0, 0, -7).Format(format)

			case "month":
				then = now.AddDate(0, -1, 0).Format(format)

			case "year":
				then = now.AddDate(-1, 0, 0).Format(format)

			default:
				if filter == "label" {
					klog.V(7).Info("filter is label. Operator is @>.")
					operator = "@>"
				} else {
					if _, ok := arrayProperties[filter]; ok {
						klog.V(7).Info("filter ", filter, " is present in arrayProperties. Using operator ?|.")
						operator = "?|"
					} else {
						klog.V(7).Info("filter is neither label nor in arrayProperties: ", filter)
						operator = ""
					}
				}
				then = val
			}
			// Add the value and operator to map
			updateOpValueMap(operator, opValueMap, then)
		}
	}
	return opValueMap
}

// Labels are sorted alphabetically to ensure consistency, then encoded in a
// string with the following format.
// key1:value1; key2:value2; ...
func formatLabels(labels map[string]interface{}) string {
	keys := make([]string, 0)
	labelStrings := make([]string, 0)
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		labelStrings = append(labelStrings, fmt.Sprintf("%s=%s", k, labels[k]))
	}
	return strings.Join(labelStrings, "; ")
}

// Encode array into a single string with the format.
//  value1; value2; ...
func formatArray(itemlist []interface{}) string {
	keys := make([]string, len(itemlist))
	for i, k := range itemlist {
		keys[i] = convertToString(k)
	}
	sort.Strings(keys)
	return strings.Join(keys, "; ")
}

// Convert interface to string format
func convertToString(data interface{}) string {
	var item string
	switch v := data.(type) {
	case string:
		item = strings.ToLower(v)
	case bool:
		item = strconv.FormatBool(v)
	case float64:
		item = strconv.FormatInt(int64(v), 10)
	default:
		klog.Warningf("Error formatting property with type: %+v\n", reflect.TypeOf(v))
	}
	return item
}

func formatDataMap(data map[string]interface{}) map[string]interface{} {
	item := make(map[string]interface{})
	for key, value := range data {
		switch v := value.(type) {
		case string:
			item[key] = v //strings.ToLower(v)
		case bool:
			item[key] = strconv.FormatBool(v)
		case float64:
			item[key] = strconv.FormatInt(int64(v), 10)
		case map[string]interface{}:
			item[key] = formatLabels(v)
		case []interface{}:
			item[key] = formatArray(v)
		default:
			klog.Warningf("Error formatting property with key: %+v  type: %+v\n", key, reflect.TypeOf(v))
			continue
		}
	}
	return item
}

// helper function to point values in string  array
func pointerToStringArray(pointerArray []*string) []string {

	values := make([]string, len(pointerArray))
	for i, val := range pointerArray {
		values[i] = *val
	}
	return values
}

func WhereClauseFilter(input *model.SearchInput) []exp.Expression {
	var whereDs []exp.Expression

	if input.Keywords != nil && len(input.Keywords) > 0 {
		// Sample query: SELECT COUNT("uid") FROM "search"."resources", jsonb_each_text("data")
		// WHERE (("value" LIKE '%dns%') AND ("data"->>'kind' ILIKE ANY ('{"pod","deployment"}')))
		keywords := pointerToStringArray(input.Keywords)
		for _, key := range keywords {
			key = "%" + key + "%"
			whereDs = append(whereDs, goqu.L(`"value"`).Like(key).Expression())
		}
	}
	if input.Filters != nil {
		for _, filter := range input.Filters {
			if len(filter.Values) > 0 {
				values := pointerToStringArray(filter.Values)
				// If property is of array type like label, remove the equal sign in it and use colon
				// - to be similar to how it is stored in the database
				if _, ok := arrayProperties[filter.Property]; ok {
					cleanedVal := make([]string, len(values))
					for i, val := range values {
						labels := strings.Split(val, "=")
						if len(labels) > 1 {
							cleanedVal[i] = fmt.Sprintf(`{"%s":"%s"}`, labels[0], labels[1])
						} else {
							//// If property is of array type, format it as an array for easy searching
							cleanedVal[i] = fmt.Sprintf(`%s`, labels[0])
						}
					}
					values = cleanedVal
				}

				// Check if value is a number or date and get the cleaned up value
				opDateValueMap := getOperatorAndNumDateFilter(filter.Property, values)

				//Sort map according to keys - This is for the ease/stability of tests when there are multiple operators
				keys := getKeys(opDateValueMap)
				sort.Strings(keys)

				var operatorWhereDs []exp.Expression //store all the clauses for this filter together
				for _, operator := range keys {
					operatorWhereDs = append(operatorWhereDs,
						getWhereClauseExpression(filter.Property, operator, opDateValueMap[operator])...)
				}
				whereDs = append(whereDs, goqu.Or(operatorWhereDs...)) //Join all the clauses with OR

			} else {
				klog.Warningf("Ignoring filter [%s] because it has no values", filter.Property)
			}
		}
	}

	return whereDs
}

func getKeys(stringArrayMap map[string][]string) []string {
	i := 0
	keys := make([]string, len(stringArrayMap))
	for k := range stringArrayMap {
		keys[i] = k
		i++
	}
	return keys
}

// Set limit for queries
func (s *SearchResult) setLimit() int {
	var limit int
	if s.input != nil && s.input.Limit != nil && *s.input.Limit > 0 {
		limit = *s.input.Limit
	} else if s.input != nil && s.input.Limit != nil && *s.input.Limit == -1 {
		klog.Warning("No limit set. Fetching all results.")
	} else {
		limit = config.Cfg.QueryLimit
	}
	return limit
}
