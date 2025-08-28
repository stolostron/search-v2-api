// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	klog "k8s.io/klog/v2"
)

type SearchRelatedResult struct {
	Kind  string                   `json:"kind"`
	Count *int                     `json:"count"`
	Items []map[string]interface{} `json:"items"`
}

// func (s *SearchRelatedResult) Count() int {
// 	return 0
// }
// func (s *SearchRelatedResult) Items() []map[string]interface{} {
// 	return nil
// }

// Builds the database query to get relationships for the items in the search result.
func (s *SearchResult) buildRelationsQuery() {
	/**
	Example query to find relations between resources - accepts an array of uids
	=============================================================================
	SELECT "uid", "kind", MIN("level") AS "level" FROM
	(
		SELECT "level", unnest(array[sourceid, destid, concat('cluster__',cluster)]) AS "uid",
	unnest(array[sourcekind, destkind, 'Cluster']) AS "kind"
		FROM (
			WITH RECURSIVE search_graph(level, sourceid, destid,  sourcekind, destkind, cluster) AS
			(
	         	SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster"
			 	FROM "search"."edges" AS "e"
			 	WHERE (("destid" IN ('local-cluster/108a77a2-159c-4621-ae1e-7a3649000ebc' ))
		     	UNION ALL
	         	SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster"
			 	FROM "search"."edges" AS "e"
			 	WHERE (("sourceid" IN ('local-cluster/108a77a2-159c-4621-ae1e-7a3649000ebc' ))
	        )
					   UNION
			 (SELECT level+1 AS "level", "e"."sourceid", "e"."destid", "e"."sourcekind", "e"."destkind", "e"."cluster"
			  FROM "search"."edges" AS "e"
			  INNER JOIN "search_graph" AS "sg"
			  ON (("sg"."destid" IN ("e"."sourceid", "e"."destid")) OR
				  ("sg"."sourceid" IN ("e"."sourceid", "e"."destid"))
				 )
	 		  WHERE (("e"."destkind" != 'Node') AND
					 ("sg"."level" <=3)
	 				)
			 )
			) SELECT DISTINCT "level", "sourceid", "destid", "sourcekind", "destkind", "cluster" FROM "search_graph"
		) AS "search_graph"
	) AS "combineIds"
	WHERE (("level" <=3)
	AND ("uid" NOT IN ('local-cluster/108a77a2-159c-4621-ae1e-7a3649000ebc')))
	GROUP BY "uid", "kind"
	-- union -- This is added if `kind:Cluster` is present in search term
	-- select uid as uid, data->>'kind' as kind, 1 AS "level" FROM search.resources where cluster IN ('local-cluster')
	*/
	s.setDepth()
	whereDs := []exp.Expression{
		goqu.C("level").Lte(s.level), // Add filter to select up to level (default 3) relationships
		goqu.C("uid").NotIn(s.uids)}  // Add filter to avoid selecting the search object itself

	//Non-recursive term SELECT CLAUSE
	schema := goqu.S("search")
	selectBase := []interface{}{goqu.L("1").As("level"), "sourceid", "destid", "sourcekind", "destkind", "cluster",
		goqu.L("array[sourceid, destid]").As("path")}

	//Recursive term SELECT CLAUSE
	selectNext := []interface{}{goqu.L("level+1").As("level"), "e.sourceid", "e.destid", "e.sourcekind",
		"e.destkind", "e.cluster", "path"}

	//Combine both source and dest ids and source and dest kinds into one column using UNNEST function
	selectCombineIds := []interface{}{goqu.C("level"),
		goqu.L("unnest(array[sourceid, destid, concat('cluster__',cluster)])").As("uid"),
		goqu.L("unnest(array[sourcekind, destkind, 'Cluster'])").As("kind"), "path"}

	//Final select statement
	selectFinal := []interface{}{goqu.C("uid"), goqu.C("kind"), goqu.MIN("level").As("level"), goqu.C("path")}

	//GROUPBY CLAUSE
	groupBy := []interface{}{goqu.C("uid"), goqu.C("kind"), goqu.C("path")}

	srcDestIds := []interface{}{goqu.I("e.sourceid"), goqu.I("e.destid")}
	excludeResources := []interface{}{"Node", "Channel"}

	// Non-recursive term
	baseSource := goqu.From(schema.Table("edges").As("e")).
		Select(selectBase...).
		Where(goqu.Ex{"sourceid": s.uids})
	baseDest := goqu.From(schema.Table("edges").As("e")).
		Select(selectBase...).
		Where(goqu.Ex{"destid": s.uids})
	baseTerm := baseSource.UnionAll(baseDest)

	// Recursive term
	recursiveTerm := goqu.From(schema.Table("edges").As("e")).
		InnerJoin(goqu.T("search_graph").As("sg"),
			goqu.On(goqu.ExOr{"sg.destid": srcDestIds, "sg.sourceid": srcDestIds})).
		Select(selectNext...).
		// Limiting up to default level 3 as it should suffice for application relations
		Where(goqu.Ex{"sg.level": goqu.Op{"Lte": s.level},
			// Avoid getting nodes and channels in recursion to prevent pulling all relations for node and channel
			"e.destkind":   goqu.Op{"neq": excludeResources},
			"e.sourcekind": goqu.Op{"neq": excludeResources}})
	var searchGraphQ *goqu.SelectDataset

	if s.level > 1 {
		klog.V(5).Infof("Search term includes applications or level set by user. Level: %d", s.level)
		// Recursive query. Refer: https://www.postgresqltutorial.com/postgresql-tutorial/postgresql-recursive-query/
		searchGraphQ = goqu.From("search_graph").
			WithRecursive("search_graph(level, sourceid, destid,  sourcekind, destkind, cluster, path)",
				baseTerm.
					Union(recursiveTerm)).
			SelectDistinct("level", "sourceid", "destid", "sourcekind", "destkind", "cluster", "path")
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
		relQuery = relQuery.Union(clusterSelectTerm).As("related")
	}
	relQuery = goqu.From(relQuery.As("related")).Select("related.uid", "related.kind",
		"related.level", "related.path")
	relQueryInnerJoin := relQuery.InnerJoin(goqu.S("search").Table("resources"),
		goqu.On(goqu.Ex{"related.uid": goqu.L(`"resources".uid`)}))
	//RBAC CLAUSE
	relQueryWithRbac := relQueryInnerJoin //without RBAC
	withoutRBACSql, _, withoutRBACSqlErr := relQueryInnerJoin.ToSQL()
	klog.V(5).Info("Relations query before RBAC:", withoutRBACSql, withoutRBACSqlErr)

	//get user info for logging
	_, userInfo := rbac.GetCache().GetUserUID(s.context)

	// if one of them is not nil, userData is not empty
	if s.userData.CsResources != nil || s.userData.NsResources != nil || s.userData.ManagedClusters != nil {
		// add rbac
		relQueryWithRbac = relQueryInnerJoin.Where(buildRbacWhereClause(s.context, s.userData, userInfo))
	} else {
		s.checkErrorBuildingQuery(fmt.Errorf("RBAC clause is required! None found for relations query %+v for user %s with uid %s ",
			s.input, userInfo.Username, userInfo.UID), "Error building search relations query")
		return
	}
	sql, params, err := relQueryWithRbac.ToSQL()

	if err != nil {
		klog.Error("Error creating relation query", err)
	} else {
		s.query = sql
		s.params = params
		klog.V(7).Info("Relations query: ", s.query)
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
		selectDs := ds.Select(goqu.C("uid").As("uid"), goqu.L("data->>'kind'").As("kind"), goqu.L("1").As("level"),
			goqu.L("array[]::text[]").As("path"))

		//WHERE CLAUSE - Do we need to add clauses here?

		//LIMIT CLAUSE - Do we need limit here?
		// limit := config.Cfg.QueryLimit

		return selectDs.Where(goqu.C("cluster").In(clusterNames))
	} else {
		return nil
	}
}

// Builds the query to get resource data from the relationships UIDs.
func (s *SearchResult) buildQueryToGetItemsFromUIDs() {
	klog.V(3).Infof("Building query to get items for [%d] uids.\n", len(s.uids))
	var params []interface{}
	var sql string
	var err error
	// define schema table
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)

	// SELECT CLAUSE
	selectDs := ds.Select("uid", "cluster", "data")

	// WHERE CLAUSE
	whereDs := []exp.Expression{goqu.C("uid").In(s.uids)} // Add filter to avoid selecting the search object itself

	// LIMIT CLAUSE
	limit := s.setLimit()

	// Get the query
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

func (s *SearchResult) getRelationResolvers(ctx context.Context) []SearchRelatedResult {
	klog.V(3).Infof("Resolving relationships for [%d] uids.\n", len(s.uids))
	relatedSearch := []SearchRelatedResult{}

	// defining variables
	relatedMap := map[string][]string{} // Map to store relations
	currSearchUidsMap := map[string]struct{}{}
	for _, uid := range PointerToStringArray(s.uids) {
		currSearchUidsMap[uid] = struct{}{}
	}
	// Maps what each result is related to
	resultToCurrSearchUidsMap := map[string][]string{} // Map to store related results to current search UIDs
	if s.context == nil {
		s.context = ctx
	}
	// Build the relations query
	s.buildRelationsQuery()
	relations, relQueryError := s.pool.Query(s.context, s.query, s.params...) // how to deal with defaults.
	if relQueryError != nil {
		klog.Errorf("Error while executing getRelations query. Error :%s", relQueryError.Error())
		return relatedSearch
	}

	if relations != nil {
		defer relations.Close()
		processedUIDs := map[string]struct{}{}
		// iterating through resulting rows and scaning data, destid  and destkind
		for relations.Next() {
			var kind, uid string
			var path []string
			var level int
			relatedResultError := relations.Scan(&uid, &kind, &level, &path)

			if relatedResultError != nil {
				klog.Errorf("Error %s retrieving rows for relationships:%s", relatedResultError.Error(), relations)
				continue
			}
			// Getting path can bring duplicate uids - Avoid duplicates by discarding already processed uids
			if _, present := processedUIDs[uid]; !present {
				processedUIDs[uid] = struct{}{}
				// Store result in a map
				s.updateKindMap(uid, kind, relatedMap)
			}
			// Store result->currentSearchUID relation
			s.updResultToCurrSearchUidsMap(uid, currSearchUidsMap, resultToCurrSearchUidsMap, path)
		}
	}
	// get uids for related items that match the relatedKind filter.
	s.filterRelatedUIDs(relatedMap)

	// if no relatedKind uids are present - return empty related Search
	if len(s.uids) > 0 {
		// Build query to get full item data from s.uids
		s.buildQueryToGetItemsFromUIDs()
		items, err := s.resolveItems() // Fetch the related items
		if err != nil {
			klog.Warning("Error resolving related items.", err)
			return []SearchRelatedResult{}
		}

		// Convert to format of the relationships resolver []SearchRelatedResult{kind, count, items}
		relatedSearch = s.searchRelatedResultKindItems(items, resultToCurrSearchUidsMap)

		klog.V(6).Info("RelatedSearch Result: ", relatedSearch)
	} else {
		klog.Warning("No UIDs matched for relatedKinds: ", PointerToStringArray(s.input.RelatedKinds))
	}
	return relatedSearch
}

// Filters the related UIDs to match the relatedKinds input.
func (s *SearchResult) filterRelatedUIDs(levelsMap map[string][]string) {
	klog.V(6).Info("levelsMap in relatedKindUIDs: ", levelsMap)

	s.uids = []*string{}

	// If relatedKinds filter is empty, include all.
	if len(s.input.RelatedKinds) == 0 {
		for _, values := range levelsMap {
			s.uids = append(s.uids, stringArrayToPointer(values)...)
		}
	} else {
		// Only include UIDs of related items that match the relatedKinds filter.
		kinds := getKeys(levelsMap)
		for _, kindFilter := range s.input.RelatedKinds {
			for _, kind := range kinds {
				if strings.EqualFold(kind, *kindFilter) {
					uids := levelsMap[kind]
					sort.Strings(uids) //stabilize unit tests
					s.uids = append(s.uids, stringArrayToPointer(uids)...)
					break
				}
			}
		}
		if len(s.uids) == 0 {
			klog.Warning("No UIDs matched for relatedKinds: ", PointerToStringArray(s.input.RelatedKinds))
		}
	}

	klog.V(6).Info("Number of related UIDs after filtering relatedKinds: ", len(s.uids))
}

func (s *SearchResult) searchRelatedResultKindItems(items []map[string]interface{},
	resultToCurrSearchMap map[string][]string) []SearchRelatedResult {
	// Organize the related items by kind.
	relatedItemsByKind := map[string][]map[string]interface{}{}
	for _, currItem := range items {
		kind := currItem["kind"].(string)
		relatedUids := resultToCurrSearchMap[currItem["_uid"].(string)]
		// Add the related ids to the currently processing item
		currItem["_relatedUids"] = relatedUids
		kindItemList := relatedItemsByKind[kind]
		relatedItemsByKind[kind] = append(kindItemList, currItem)
	}

	// Generate result for each kind.
	result := make([]SearchRelatedResult, 0)
	for kind, items := range relatedItemsByKind {
		count := len(items)
		result = append(result, SearchRelatedResult{Kind: kind, Items: items, Count: &count})
	}
	return result
}

func (s *SearchResult) updateKindMap(uid string, kind string, levelMap map[string][]string) {
	uids := levelMap[kind]
	uids = append(uids, uid)

	levelMap[kind] = uids
}

// Maps the related result uids to the uids in the search result set.
func (s *SearchResult) updResultToCurrSearchUidsMap(resultUid string, currSearchUidsMap map[string]struct{},
	resultToCurrSearchUidsMap map[string][]string, path []string) {

	for _, relatedUid := range path {
		if _, ok := currSearchUidsMap[relatedUid]; ok {
			klog.V(9).Infof("uid %s is valid and part of the current search. Number of relatedUids: %d",
				relatedUid, len(resultToCurrSearchUidsMap[resultUid]))
			if _, found := resultToCurrSearchUidsMap[resultUid]; !found {
				resultToCurrSearchUidsMap[resultUid] = []string{relatedUid}
			} else {
				// if the relatedUid is not already added, append it
				if !CheckIfInArray(resultToCurrSearchUidsMap[resultUid], relatedUid) {
					resultToCurrSearchUidsMap[resultUid] = append(resultToCurrSearchUidsMap[resultUid], relatedUid)
					klog.V(9).Infof("uid %s is newly mapped to uids %+v", resultUid,
						resultToCurrSearchUidsMap[resultUid])
				}
			}
			klog.V(9).Infof("uid %s is  related to uids %+v.", resultUid, resultToCurrSearchUidsMap[resultUid])
			// Need to process only one uid in the path as it is either the sourceid or the destid that can be related
			break
		}
	}
}

func (s *SearchResult) setDepth() {
	// This level will come into effect only in case of Application relations.
	// For normal searches, we go only upto level 1. This can be changed later, if necessary.
	s.level = config.Cfg.RelationLevel
	//The level can be parameterized later, if needed

	//Set level
	if s.searchApplication() && s.level == 0 {
		s.level = 3 // If search involves applications and level is not explicitly set by user, set to 3
		klog.V(3).Infof("Search includes applications. Level set to %d.", s.level)
	} else if s.level == 0 {
		s.level = 1 // If level is not explicitly set by user, set to 1
		klog.V(6).Infof("Default value for level set: %d.", s.level)
	}
}

// Check if the search input filters contain Application - either in kind field or relatedKinds
func (s *SearchResult) searchApplication() bool {
	srchString := "Application"
	for _, filter := range s.input.Filters {
		for _, val := range filter.Values {
			if strings.EqualFold(*val, srchString) {
				klog.V(9).Info("searchApplication returns true. Search filter includes application")
				return true
			}
		}
	}
	for _, relKind := range s.input.RelatedKinds {
		if strings.EqualFold(*relKind, srchString) {
			klog.V(9).Info("searchApplication returns true. relatedkinds includes application")
			return true
		}
	}
	klog.V(9).Info("searchApplication returns false. relatedkind/filter doesn't include application")
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
