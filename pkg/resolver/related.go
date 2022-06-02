// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/stolostron/search-v2-api/pkg/config"
	klog "k8s.io/klog/v2"
)

type SearchRelatedResult struct {
	// input *model.SearchInput //nolint: unused,structcheck
	Kind  string                   `json:"kind"`
	Count *int                     `json:"count"`
	Items []map[string]interface{} `json:"items"`
}

// func (s *SearchRelatedResult) Count() int {
// 	klog.Info("TODO: Resolve SearchRelatedResult: Count() - model/related.go")
// 	return 0
// }

// func (s *SearchRelatedResult) Kind() string {
// 	klog.Info("TODO: Resolve SearchRelatedResult: Kind()  - model/related.go")
// 	return "TODO:Kind"
// }

// func (s *SearchRelatedResult) Items() []map[string]interface{} {
// 	klog.Info("TODO: Resolve SearchRelatedResult: Items() - model/related.go")
// 	return nil
// }
func (s *SearchResult) buildRelationsQuery() {
	// Example query to find relations between resources - accepts an array of uids
	// =============================================================================
	// 	SELECT "uid", "kind", MIN("level") AS "level" FROM
	// (
	// 	SELECT "level", unnest(array[sourceid, destid, concat('cluster__',cluster)]) AS "uid",
	// unnest(array[sourcekind, destkind, 'Cluster']) AS "kind"
	// 	FROM (
	// 		WITH RECURSIVE search_graph(level, sourceid, destid,  sourcekind, destkind, cluster) AS
	// 		(SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster"
	// 		 FROM "search"."all_edges" AS "e"
	// 		 WHERE (("destid" IN ('local-cluster/108a77a2-159c-4621-ae1e-7a3649000ebc' )) OR
	// 					("sourceid" IN ('local-cluster/108a77a2-159c-4621-ae1e-7a3649000ebc'))
	// 			   )
	// 				   UNION
	// 		 (SELECT level+1 AS "level", "e"."sourceid", "e"."destid", "e"."sourcekind", "e"."destkind", "e"."cluster"
	// 		  FROM "search"."all_edges" AS "e"
	// 		  INNER JOIN "search_graph" AS "sg"
	// 		  ON (("sg"."destid" IN ("e"."sourceid", "e"."destid")) OR
	// 			  ("sg"."sourceid" IN ("e"."sourceid", "e"."destid"))
	// 			 )
	//  		  WHERE (("e"."destkind" != 'Node') AND
	// 				 ("sg"."level" <=3)
	//  				)
	// 		 )
	// 		) SELECT DISTINCT "level", "sourceid", "destid", "sourcekind", "destkind", "cluster" FROM "search_graph"
	// 	) AS "search_graph"
	// ) AS "combineIds"
	// WHERE (("level" <=3)
	// AND ("uid" NOT IN ('local-cluster/108a77a2-159c-4621-ae1e-7a3649000ebc')))
	// GROUP BY "uid", "kind"
	// -- union -- This is added if `kind:Cluster` is present in search term
	// -- select uid as uid, data->>'kind' as kind, 1 AS "level" FROM search.resources where cluster IN ('local-cluster')

	level := s.setDepth()
	whereDs := []exp.Expression{
		goqu.C("level").Lte(level),  // Add filter to select up to level (default 3) relationships
		goqu.C("uid").NotIn(s.uids)} // Add filter to avoid selecting the search object itself

	//Non-recursive term SELECT CLAUSE
	schema := goqu.S("search")
	selectBase := []interface{}{goqu.L("1").As("level"), "sourceid", "destid", "sourcekind", "destkind", "cluster"}

	//Recursive term SELECT CLAUSE
	selectNext := []interface{}{goqu.L("level+1").As("level"), "e.sourceid", "e.destid", "e.sourcekind",
		"e.destkind", "e.cluster"}

	//Combine both source and dest ids and source and dest kinds into one column using UNNEST function
	selectCombineIds := []interface{}{goqu.C("level"),
		goqu.L("unnest(array[sourceid, destid, concat('cluster__',cluster)])").As("uid"),
		goqu.L("unnest(array[sourcekind, destkind, 'Cluster'])").As("kind")}

	//Final select statement
	selectFinal := []interface{}{goqu.C("uid"), goqu.C("kind"), goqu.MIN("level").As("level")}

	//GROUPBY CLAUSE
	groupBy := []interface{}{goqu.C("uid"), goqu.C("kind")}

	srcDestIds := []interface{}{goqu.I("e.sourceid"), goqu.I("e.destid")}

	// Non-recursive term
	baseTerm := goqu.From(schema.Table("all_edges").As("e")).
		Select(selectBase...).
		Where(goqu.ExOr{"sourceid": (s.uids), "destid": (s.uids)})

	// Recursive term
	recursiveTerm := goqu.From(schema.Table("all_edges").As("e")).
		InnerJoin(goqu.T("search_graph").As("sg"),
			goqu.On(goqu.ExOr{"sg.destid": srcDestIds, "sg.sourceid": srcDestIds})).
		Select(selectNext...).
		// Limiting upto default level 3 as it should suffice for application relations
		Where(goqu.Ex{"sg.level": goqu.Op{"Lte": level},
			// Avoid getting nodes in recursion to prevent pulling all relations for node
			"e.destkind": goqu.Op{"neq": "Node"}})
	var searchGraphQ *goqu.SelectDataset

	if level > 1 {
		klog.V(5).Infof("Search term includes applications. Level: %d", level)
		// Recursive query. Refer: https://www.postgresqltutorial.com/postgresql-tutorial/postgresql-recursive-query/
		searchGraphQ = goqu.From("search_graph").
			WithRecursive("search_graph(level, sourceid, destid,  sourcekind, destkind, cluster)",
				baseTerm.
					Union(recursiveTerm)).
			SelectDistinct("level", "sourceid", "destid", "sourcekind", "destkind", "cluster")
	} else {
		searchGraphQ = baseTerm // Query without recursion since it is only level 1
	}
	combineIds := goqu.From(searchGraphQ.As("search_graph")).Select(selectCombineIds...)
	var relQuery *goqu.SelectDataset

	relQuery = goqu.From(combineIds.As("combineIds")).
		Select(selectFinal...).
		Where(whereDs...).
		GroupBy(groupBy...)

	// Check if the uids include cluster uids. This will be true if search term includes `kind: Cluster`
	// Since there are no direct edges between cluster node and other nodes,
	// add a union to the relation query to get all resources in the clusters
	clusterSelectTerm := s.selectIfClusterUIDPresent()
	if clusterSelectTerm != nil {
		relQuery = relQuery.Union(clusterSelectTerm)
	}
	sql, params, err := relQuery.ToSQL()

	if err != nil {
		klog.Error("Error creating relation query", err)
	} else {
		s.query = sql
		s.params = params
		klog.V(3).Info("Relations query: ", s.query)
	}
}

// Check if clusters are part of the search input `kind: Cluster`
func (s *SearchResult) selectIfClusterUIDPresent() *goqu.SelectDataset {
	var clusterNames []string
	for _, uid := range s.uids { // check if cluster uid is in s.uids
		if strings.HasPrefix(*uid, "cluster__") {
			clusterName := strings.TrimPrefix(*uid, "cluster__") // get the cluster name
			clusterNames = append(clusterNames, clusterName)
		}
	}
	if len(clusterNames) > 0 {
		// Sample query: select uid as uid, data->>'kind' as kind, 1 AS "level" FROM search.resources
		// where cluster IN ('local-cluster', 'sv-remote-1')
		//define schema table:
		schemaTable := goqu.S("search").Table("resources")
		ds := goqu.From(schemaTable)

		//SELECT CLAUSE
		selectDs := ds.Select(goqu.C("uid").As("uid"), goqu.L("data->>'kind'").As("kind"), goqu.L("1").As("level"))

		//WHERE CLAUSE - Do we need to add clauses here?

		//LIMIT CLAUSE - Do we need limit here?
		// limit := config.Cfg.QueryLimit

		return selectDs.Where(goqu.C("cluster").In(clusterNames))
	} else {
		return nil
	}
}

func (s *SearchResult) buildRelatedKindsQuery() {
	klog.V(3).Infof("Resolving relationships for [%d] uids.\n", len(s.uids))
	var params []interface{}
	var sql string
	var err error
	//define schema table:
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)

	//SELECT CLAUSE
	selectDs := ds.Select("uid", "cluster", "data")

	//WHERE CLAUSE
	whereDs := []exp.Expression{goqu.C("uid").In(s.uids)} // Add filter to avoid selecting the search object itself

	//LIMIT CLAUSE
	limit := s.setLimit()

	//Get the query
	if limit != 0 {
		sql, params, err = selectDs.Where(whereDs...).Limit(uint(limit)).ToSQL()
	} else {
		sql, params, err = selectDs.Where(whereDs...).ToSQL()
	}
	if err != nil {
		klog.Errorf("Error building SearchRelatedKinds query: %s", err.Error())
		return
	}
	klog.V(3).Infof("SearchRelatedKinds query: %s\nargs: %s", sql, params)
	s.query = sql
	s.params = params
}

func (s *SearchResult) getRelations() []SearchRelatedResult {
	klog.V(3).Infof("Resolving relationships for [%d] uids.\n", len(s.uids))
	relatedSearch := []SearchRelatedResult{}

	//defining variables
	relatedMap := map[string][]string{} // Map to store relations

	// Build the relations query
	s.buildRelationsQuery()

	relations, relQueryError := s.pool.Query(context.TODO(), s.query, s.params...) // how to deal with defaults.
	if relQueryError != nil {
		klog.Errorf("Error while executing getRelations query. Error :%s", relQueryError.Error())
		return relatedSearch
	}

	defer relations.Close()

	// iterating through resulting rows and scaning data, destid  and destkind
	for relations.Next() {
		var kind, uid string
		var level int
		relatedResultError := relations.Scan(&uid, &kind, &level)

		if relatedResultError != nil {
			klog.Errorf("Error %s retrieving rows for relationships:%s", relatedResultError.Error(), relations)
		}
		s.updateKindMap(uid, kind, relatedMap) // Store result in a map
	}

	// retrieve provided kind filters only
	if len(s.input.RelatedKinds) > 0 {
		s.relatedKindUIDs(relatedMap) // get uids for provided kinds
		// Build Related kinds query
		s.buildRelatedKindsQuery()
		items, err := s.resolveItems() // Fetch the related kind items
		if err != nil {
			klog.Warning("Error resolving relatedKind items", err)
		} else { // Convert to (kind, items) format
			relatedSearch = s.searchRelatedResultKindItems(items)
		}
	} else { // Retrieve kind and count of related items
		relatedSearch = s.searchRelatedResultKindCount(relatedMap)
	}
	klog.V(5).Info("relatedSearch: ", relatedSearch)
	return relatedSearch
}

func (s *SearchResult) relatedKindUIDs(levelsMap map[string][]string) {
	relatedKinds := pointerToStringArray(s.input.RelatedKinds)
	s.uids = []*string{}
	for _, kind := range relatedKinds {
		s.uids = append(s.uids, stringArrayToPointer(levelsMap[kind])...)
	}
}

func (s *SearchResult) searchRelatedResultKindCount(levelMap map[string][]string) []SearchRelatedResult {

	relatedSearch := make([]SearchRelatedResult, len(levelMap))

	i := 0
	//iterating and sending values to relatedSearch
	for kind, uidArray := range levelMap {
		count := len(uidArray)
		relatedSearch[i] = SearchRelatedResult{Kind: kind, Count: &count}
		i++
	}
	return relatedSearch
}

func (s *SearchResult) searchRelatedResultKindItems(items []map[string]interface{}) []SearchRelatedResult {

	relatedSearch := make([]SearchRelatedResult, len(s.input.RelatedKinds))
	relatedItems := map[string][]map[string]interface{}{}

	//iterating and sending values to relatedSearch
	for _, currItem := range items {
		currKind := currItem["kind"].(string)
		kindItemList := relatedItems[currKind]

		kindItemList = append(kindItemList, currItem)
		relatedItems[currKind] = kindItemList
	}

	i := 0
	//iterating and sending values to relatedSearch
	for kind, items := range relatedItems {
		relatedSearch[i] = SearchRelatedResult{Kind: kind, Items: items}
		i++
	}
	return relatedSearch
}

func (s *SearchResult) updateKindMap(uid string, kind string, levelMap map[string][]string) {
	s.mux.RLock() // Lock map to read
	uids := levelMap[kind]
	s.mux.RUnlock()

	uids = append(uids, uid)

	s.mux.Lock() // Lock map to write
	levelMap[kind] = uids
	s.mux.Unlock()
}

func (s *SearchResult) setDepth() int {
	// This level will come into effect only in case of Application relations.
	// For normal searches, we go only upto level 1. This can be changed later, if necessary.
	level := config.Cfg.RelationLevel
	//The level can be parameterized later, if needed

	//Set level
	if s.searchApplication() && level == 0 {
		level = 3 // If search involves applications and level is not explicitly set by user, set to 3
	} else if level == 0 {
		level = 1 // If level is not explicitly set by user, set to 1
	}
	return level
}

// Check if the search input filters contain Application - either in kind field or relatedKinds
func (s *SearchResult) searchApplication() bool {
	srchString := "Application"
	for _, filter := range s.input.Filters {
		for _, val := range filter.Values {
			if strings.EqualFold(*val, srchString) {
				return true
			}
		}
	}
	for _, relKind := range s.input.RelatedKinds {
		if strings.EqualFold(*relKind, srchString) {
			return true
		}
	}
	return false
}

func stringArrayToPointer(stringArray []string) []*string {

	values := make([]*string, len(stringArray))
	for i, val := range stringArray {
		tmpVal := val
		values[i] = &tmpVal
	}
	return values
}
