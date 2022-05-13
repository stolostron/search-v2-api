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

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/driftprogramming/pgxpoolmock"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
	db "github.com/stolostron/search-v2-api/pkg/database"
	"k8s.io/klog/v2"
)

type SearchResult struct {
	input  *model.SearchInput
	pool   pgxpoolmock.PgxPool
	uids   []*string      // List of uids from search result to be used to get relatioinships.
	wg     sync.WaitGroup // WORKAROUND: Used to serialize search query and relatioinships query.
	query  string
	params []interface{}
	//  Related []SearchRelatedResult
}

func Search(ctx context.Context, input []*model.SearchInput) ([]*SearchResult, error) {
	// For each input, create a SearchResult resolver.
	srchResult := make([]*SearchResult, len(input))
	if len(input) > 0 {
		for index, in := range input {
			srchResult[index] = &SearchResult{
				input: in,
				pool:  db.GetConnection(),
			}
		}
	}
	return srchResult, nil
}

func (s *SearchResult) Count() int {
	klog.V(2).Info("Resolving SearchResult:Count()")
	s.buildSearchQuery(context.Background(), true, false)
	count := s.resolveCount()

	return count
}

func (s *SearchResult) Items() []map[string]interface{} {
	s.wg.Add(1)
	defer s.wg.Done()
	klog.V(2).Info("Resolving SearchResult:Items()")
	s.buildSearchQuery(context.Background(), false, false)
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

	s.wg.Wait()
	var r []SearchRelatedResult

	if len(s.uids) > 0 {
		r = s.getRelations()
	} else {
		klog.Warning("No uids selected for query:Related()")
	}
	return r
}

func (s *SearchResult) Uids() {
	klog.V(2).Info("Resolving SearchResult:Uids()")
	s.buildSearchQuery(context.Background(), false, true)
	s.resolveUids()
}

func (s *SearchResult) buildSearchQuery(ctx context.Context, count bool, uid bool) {
	var limit int
	var selectDs *goqu.SelectDataset
	var whereDs []exp.Expression

	// Example query: SELECT uid, cluster, data FROM search.resources  WHERE lower(data->> 'kind') IN
	// (lower('Pod')) AND lower(data->> 'cluster') IN (lower('local-cluster')) LIMIT 10000

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
		selectDs = ds.Select("uid", "cluster", "data")
	}

	//WHERE CLAUSE
	if s.input != nil && (len(s.input.Filters) > 0 || (s.input.Keywords != nil && len(s.input.Keywords) > 0)) {
		whereDs = WhereClauseFilter(s.input)
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
	klog.V(3).Infof("Search query: %s\nargs: %s", sql, params)
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
	rows, err := s.pool.Query(context.Background(), s.query, s.params...)
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
	rows, err := s.pool.Query(context.Background(), s.query, s.params...)
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
func getOperator(values []string) (string, []string) {
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
	newValues := make([]string, len(values)) // To store cleaned up values (without operator)
	for i, value := range values {
		operatorRemovedValue := replacer.Replace(value)
		newValues[i] = operatorRemovedValue
		operator = strings.Replace(value, operatorRemovedValue, "", 1) // find operator
	}
	return operator, newValues
}

func getWhereClauseExpression(prop, operator string, values []string) exp.Expression {
	switch operator {
	case "<=":
		return goqu.L(`"data"->>?`, prop).Lte(values).Expression()
	case ">=":
		return goqu.L(`"data"->>?`, prop).Gte(values).Expression()
	case "!=":
		return goqu.L(`"data"->>?`, prop).Neq(values).Expression()
	case "!":
		return goqu.L(`"data"->>?`, prop).NotIn(values).Expression()
	case "<":
		return goqu.L(`"data"->>?`, prop).Lt(values).Expression()
	case ">":
		return goqu.L(`"data"->>?`, prop).Gt(values).Expression()
	case "=":
		return goqu.L(`"data"->>?`, prop).In(values).Expression()
	default:
		if prop == "cluster" {
			return goqu.C(prop).In(values).Expression()
		}
		return goqu.L(`"data"->>?`, prop).In(values).Expression()
	}

}

func getDateFilter(values []string) (string, []string) {
	// Expected values: {"hour", "day", "week", "month", "year"}
	newValues := make([]string, len(values))

	now := time.Now()
	operator := ">"
	for i, val := range values {
		switch val {
		case "hour":
			then := now.Add(time.Duration(-1) * time.Hour).String()
			newValues[i] = then

		case "day":
			then := now.AddDate(0, 0, -1).String()
			newValues[i] = then

		case "week":
			then := now.AddDate(0, 0, -7).String()
			newValues[i] = then

		case "month":
			then := now.AddDate(0, -1, 0).String()
			newValues[i] = then

		case "year":
			then := now.AddDate(-1, 0, 0).String()
			newValues[i] = then

		default:
			operator = ""
			newValues[i] = val
		}
	}
	return operator, newValues
}

func (s *SearchResult) getRelations() []SearchRelatedResult {
	klog.V(3).Infof("Resolving relationships for [%d] uids.\n", len(s.uids))
	var whereDs []exp.Expression

	if len(s.input.RelatedKinds) > 0 {
		relatedKinds := pointerToStringArray(s.input.RelatedKinds)
		whereDs = append(whereDs, goqu.C("destkind").In(relatedKinds).Expression())
		klog.Warning("TODO: The relationships query must use the provided kind filters effectively.")
	}
	//The level can be parameterized later, if needed, for applications
	whereDs = append(whereDs, goqu.C("level").Eq(1)) // Add filter to select only level 1 relationships

	//defining variables
	items := []map[string]interface{}{}
	var kindSlice []string
	var kindList []string
	var countList []int

	schema := goqu.S("search")
	selectBase := make([]interface{}, 0)
	selectBase = append(selectBase, "r.uid", "r.data", "e.destkind", "e.sourceid", "e.destid",
		goqu.L("ARRAY[r.uid]").As("path"), goqu.L("1").As("level"))

	selectNext := make([]interface{}, 0)
	selectNext = append(selectNext, "r.uid", "r.data", "e.destkind", "e.sourceid", "e.destid",
		goqu.L("sg.path||r.uid").As("path"), goqu.L("level+1").As("level"))
	// Original query to find relations between resources - accepts an array of uids
	// =============================================================================
	// relQuery := strings.TrimSpace(`WITH RECURSIVE
	// 	search_graph(uid, data, destkind, sourceid, destid, path, level)
	// 	AS (
	// 	SELECT r.uid, r.data, e.destkind, e.sourceid, e.destid, ARRAY[r.uid] AS path, 1 AS level
	// 		FROM search.resources r
	// 		INNER JOIN
	// 			search.edges e ON (r.uid = e.sourceid) OR (r.uid = e.destid)
	// 		 WHERE r.uid = ANY($1)
	// 	UNION
	// 	SELECT r.uid, r.data, e.destkind, e.sourceid, e.destid, path||r.uid, level+1 AS level
	// 		FROM search.resources r
	// 		INNER JOIN
	// 			search.edges e ON (r.uid = e.sourceid)
	// 		, search_graph sg
	// 		WHERE (e.sourceid = sg.destid OR e.destid = sg.sourceid)
	// 		AND r.uid <> all(sg.path)
	// 		AND level = 1
	// 		)
	// 	SELECT distinct ON (destid) data, destid, destkind FROM search_graph WHERE level=1 OR destid = ANY($1)`)
	sql, params, err := goqu.From("search_graph").
		WithRecursive("search_graph(uid, data, destkind, sourceid, destid, path, level)",
			goqu.From(schema.Table("resources").As("r")).InnerJoin(schema.Table("edges").As("e"),
				goqu.On(goqu.ExOr{"r.uid": []exp.IdentifierExpression{goqu.I("e.sourceid"), goqu.I("e.destid")}})).
				Select(selectBase...).
				Where(goqu.I("r.uid").In(s.uids)).
				Union(goqu.From(schema.Table("resources").As("r")).InnerJoin(schema.Table("edges").As("e"),
					goqu.On(goqu.Ex{"r.uid": goqu.I("e.sourceid")})).
					InnerJoin(goqu.T("search_graph").As("sg"),
						goqu.On(goqu.ExOr{"sg.destid": goqu.I("e.sourceid"), "sg.sourceid": goqu.I("e.destid")})).
					Select(selectNext...).
					Where(goqu.Ex{"sg.level": goqu.L("1"),
						"r.uid": goqu.Op{"neq": goqu.All("{sg.path}")}}))).
		Select("data", "destid", "destkind").Distinct("destid").
		Where(whereDs...).ToSQL()

	if err != nil {
		klog.Error("Error creating relation query", err)
		return nil
	}
	klog.V(3).Info("Relations query: ", sql)
	relations, relQueryError := s.pool.Query(context.TODO(), sql, params...) // how to deal with defaults.
	if relQueryError != nil {
		klog.Errorf("Error while executing getRelations query. Error :%s", relQueryError.Error())
	}

	defer relations.Close()

	// iterating through resulting rows and scaning data, destid and destkind
	for relations.Next() {
		var destkind, destid string
		var data map[string]interface{}
		relatedResultError := relations.Scan(&data, &destid, &destkind)
		if relatedResultError != nil {
			klog.Errorf("Error %s retrieving rows for relationships:%s", relatedResultError.Error(), relations)
		}

		// creating currItem variable to keep data and converting strings in data to lowercase
		currItem := formatDataMap(data)

		// currItem["Kind"] = destkind
		kindSlice = append(kindSlice, destkind)
		items = append(items, currItem)
	}

	// calling function to get map which contains unique values from kindSlice
	// and counts the number occurances ex: map[key:Pod, value:2] if pod occurs 2x in kindSlice
	count := printUniqueValue(kindSlice)

	//iterating over count and appending to new lists (kindList and countList)
	for k, v := range count {
		kindList = append(kindList, k)
		countList = append(countList, v)
	}

	//instantiating composite literal
	relatedSearch := make([]SearchRelatedResult, len(count))

	//iterating and sending values to relatedSearch
	for i := range kindList {
		kind := kindList[i]
		count := countList[i]
		relatedSearch[i] = SearchRelatedResult{kind, &count, items}
	}

	return relatedSearch
}

// helper function TODO: make helper.go module to store these if needed.
func printUniqueValue(arr []string) map[string]int {
	// Create a dictionary of values for each element
	dict := make(map[string]int)
	for _, num := range arr {
		dict[num] = dict[num] + 1
	}
	return dict
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
		labelStrings = append(labelStrings, fmt.Sprintf("%s:%s", k, labels[k]))
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

func pointerToStringArray(pointerArray []*string) []string {

	values := make([]string, len(pointerArray))
	for i, val := range pointerArray {
		values[i] = *val
	}
	return values
}

func stringArrayToPointer(stringArray []string) []*string {

	values := make([]*string, len(stringArray))
	for i, val := range stringArray {
		tmpVal := val
		values[i] = &tmpVal
	}
	return values
}
func WhereClauseFilter(input *model.SearchInput) []exp.Expression {
	var whereDs []exp.Expression

	if input.Keywords != nil && len(input.Keywords) > 0 {
		// Sample query: SELECT COUNT("uid") FROM "search"."resources", jsonb_each_text("data")
		// WHERE (("value" LIKE '%dns%') AND ("data"->>'kind' IN ('Pod')))
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

				operator, values := getOperator(values) // Check if value is a number and get the operator
				if operator == "" {                     //If not a number (no operator), check if values are dates
					operator, values = getDateFilter(values) // Check if value is a date and get the date value
				}
				whereDs = append(whereDs, getWhereClauseExpression(filter.Property, operator, values))
			} else {
				klog.Warningf("Ignoring filter [%s] because it has no values", filter.Property)
			}
		}
	}

	return whereDs
}

//Set limit for queries
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
