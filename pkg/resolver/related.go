// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
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
	var whereDs []exp.Expression

	//The level can be parameterized later, if needed
	whereDs = append(whereDs, goqu.C("level").Lt(4))       // Add filter to select only upto level 4 relationships
	whereDs = append(whereDs, goqu.C("iid").NotIn(s.uids)) // Add filter to avoid selecting the search object itself

	//Non-recursive term SELECT CLAUSE
	schema := goqu.S("search")
	selectBase := make([]interface{}, 0)
	selectBase = append(selectBase, goqu.L("1").As("level"), "sourceid", "destid", "sourcekind", "destkind", "cluster")

	//Recursive term SELECT CLAUSE
	selectNext := make([]interface{}, 0)
	selectNext = append(selectNext, goqu.L("level+1").As("level"), "e.sourceid", "e.destid", "e.sourcekind",
		"e.destkind", "e.cluster")

	//Combine both source and dest ids and source and dest kinds into one column using UNNEST function
	selectCombineIds := make([]interface{}, 0)
	selectCombineIds = append(selectCombineIds, goqu.C("level"),
		goqu.L("unnest(array[sourceid, destid, concat('cluster__',cluster)])").As("iid"),
		goqu.L("unnest(array[sourcekind, destkind, 'Cluster'])").As("kind"))

	//Final select statement
	selectFinal := make([]interface{}, 0)
	selectFinal = append(selectFinal, goqu.C("iid"), goqu.C("kind"), goqu.MIN("level").As("level"))

	//GROUPBY CLAUSE
	groupBy := make([]interface{}, 0)
	groupBy = append(groupBy, goqu.C("iid"), goqu.C("kind"))

	// Original query to find relations between resources - accepts an array of uids
	// =============================================================================
	srcDestIds := make([]interface{}, 0)
	srcDestIds = append(srcDestIds, goqu.I("e.sourceid"), goqu.I("e.destid"))

	// Non-recursive term
	baseTerm := goqu.From(schema.Table("all_edges").As("e")).
		Select(selectBase...).
		Where(goqu.ExOr{"sourceid": (s.uids), "destid": (s.uids)})

	// Recursive term
	recursiveTerm := goqu.From(schema.Table("all_edges").As("e")).
		InnerJoin(goqu.T("search_graph").As("sg"),
			goqu.On(goqu.ExOr{"sg.destid": srcDestIds, "sg.sourceid": srcDestIds})).
		Select(selectNext...).
		// Limiting upto level 4 as it should suffice for application relations
		Where(goqu.Ex{"sg.level": goqu.Op{"Lt": goqu.L("4")},
			// Avoid getting nodes in recursion to prevent pulling all relations for node
			"e.destkind": goqu.Op{"neq": "Node"}})

	// Recursive query. Refer: https://www.postgresqltutorial.com/postgresql-tutorial/postgresql-recursive-query/
	searchGraphQ := goqu.From("search_graph").
		WithRecursive("search_graph(level, sourceid, destid,  sourcekind, destkind, cluster)",
			baseTerm.
				Union(recursiveTerm)).
		SelectDistinct("level", "sourceid", "destid", "sourcekind", "destkind", "cluster")

	combineIds := goqu.From(searchGraphQ.As("search_graph")).Select(selectCombineIds...)

	relQuery := goqu.From(combineIds.As("combineIds")).
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
		// Sample query: select uid as iid, data->>'kind' as kind, 1 AS "level" FROM search.resources
		// where cluster IN ('local-cluster', 'sv-remote-1')
		//define schema table:
		schemaTable := goqu.S("search").Table("resources")
		ds := goqu.From(schemaTable)

		//SELECT CLAUSE
		selectDs := ds.Select(goqu.C("uid").As("iid"), goqu.L("data->>'kind'").As("kind"), goqu.L("1").As("level"))

		//WHERE CLAUSE

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
	var whereDs []exp.Expression
	whereDs = append(whereDs, goqu.C("uid").In(s.uids)) // Add filter to avoid selecting the search object itself

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

	//defining variables
	level1Map := map[string][]string{}    // Map to store level 1 relations
	allLevelsMap := map[string][]string{} // Map to store all relations
	var keepAllLevels bool

	// Build the relations query
	s.buildRelationsQuery()

	relations, relQueryError := s.pool.Query(context.TODO(), s.query, s.params...) // how to deal with defaults.
	if relQueryError != nil {
		klog.Errorf("Error while executing getRelations query. Error :%s", relQueryError.Error())
		return nil
	}

	defer relations.Close()

	// iterating through resulting rows and scaning data, destid  and destkind
	for relations.Next() {
		var kind, iid string
		var level int
		relatedResultError := relations.Scan(&iid, &kind, &level)

		if relatedResultError != nil {
			klog.Errorf("Error %s retrieving rows for relationships:%s", relatedResultError.Error(), relations)
			return nil
		}
		if level == 1 { // update map if level is 1
			s.updateKindMap(iid, kind, level1Map)
		}
		s.updateKindMap(iid, kind, allLevelsMap)

		// Turn on keepAllLevels if the kind is Application or Subscription
		if kind == "Application" || kind == "Subscription" {
			keepAllLevels = true
		}
	}
	klog.V(5).Info("keepAllLevels? ", keepAllLevels)
	var relatedSearch []SearchRelatedResult

	// retrieve provided kind filters only
	if len(s.input.RelatedKinds) > 0 {
		if keepAllLevels {
			s.relatedKindUIDs(allLevelsMap) // get uids for provided kinds
		} else {
			s.relatedKindUIDs(level1Map) // get uids for provided kinds
		}
		// Build Related kinds query
		s.buildRelatedKindsQuery()
		items, err := s.resolveItems() // Fetch the related kind items
		if err != nil {
			klog.Warning("Error resolving relatedKind items", err)
		} else { // Convert to (kind, items) format
			relatedSearch = s.searchRelatedResultKindItems(items)
		}
	} else { // Retrieve all related resources
		if keepAllLevels {
			relatedSearch = s.searchRelatedResultKindCount(allLevelsMap)
		} else {
			relatedSearch = s.searchRelatedResultKindCount(level1Map)
		}
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
	for kind, iidArray := range levelMap {
		count := len(iidArray)
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

func (s *SearchResult) updateKindMap(iid string, kind string, levelMap map[string][]string) {
	s.mux.RLock() // Lock map to read
	iids := levelMap[kind]
	s.mux.RUnlock()

	iids = append(iids, iid)

	s.mux.Lock() // Lock map to write
	levelMap[kind] = iids
	s.mux.Unlock()
}
