// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/rbac"
)

func Test_SearchResolver_Relationships(t *testing.T) {
	config.Cfg.RelationLevel = 3
	var resultList []*string

	uid1 := "local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd"
	uid2 := "local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b"

	resultList = append(resultList, &uid1, &uid2)

	// take the uids from above as input
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "uid", Values: resultList}}}
	ud := rbac.UserData{}

	resolver, mockPool2 := newMockSearchResolver(t, searchInput, resultList, &ud, nil)

	relQuery := strings.TrimSpace(`SELECT "related"."uid", "related"."kind", "related"."level" FROM (SELECT "uid", "kind", MIN("level") AS "level" FROM (SELECT "level", unnest(array[sourceid, destid, concat('cluster__',cluster)]) AS "uid", unnest(array[sourcekind, destkind, 'Cluster']) AS "kind" FROM (WITH RECURSIVE search_graph(level, sourceid, destid,  sourcekind, destkind, cluster) AS (SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster" FROM "search"."edges" AS "e" WHERE (("destid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b')) OR ("sourceid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b'))) UNION (SELECT level+1 AS "level", "e"."sourceid", "e"."destid", "e"."sourcekind", "e"."destkind", "e"."cluster" FROM "search"."edges" AS "e" INNER JOIN "search_graph" AS "sg" ON (("sg"."destid" IN ("e"."sourceid", "e"."destid")) OR ("sg"."sourceid" IN ("e"."sourceid", "e"."destid"))) WHERE (("e"."destkind" NOT IN ('Node', 'Channel')) AND ("e"."sourcekind" NOT IN ('Node', 'Channel')) AND ("sg"."level" <= 3)))) SELECT DISTINCT "level", "sourceid", "destid", "sourcekind", "destkind", "cluster" FROM "search_graph") AS "search_graph") AS "combineIds" WHERE (("level" <= 3) AND ("uid" NOT IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b'))) GROUP BY "uid", "kind") AS "related" INNER JOIN "search"."resources" ON ("related"."uid" = "resources".uid) WHERE (("cluster" = ANY ('{}')) OR ((data->>'_hubClusterResource' = 'true') AND NULL))`)

	mockRows := newMockRowsWithoutRBAC("./mocks/mock-rel-1.json", searchInput, "", 0)
	mockPool2.EXPECT().Query(gomock.Any(),
		gomock.Eq(relQuery),
		gomock.Eq([]interface{}{}),
	).Return(mockRows, nil)

	result := resolver.Related(context.Background()) // this should return a relatedResults object
	resultKinds := make([]*string, len(result))
	for i, data := range result {
		kind := data.Kind
		resultKinds[i] = &kind
	}

	expectedKinds := make([]*string, len(mockRows.mockData))
	for i, data := range mockRows.mockData {
		destKind, _ := data["kind"].(string)
		expectedKinds[i] = &destKind
	}
	// Verify expected and result kinds
	AssertStringArrayEqual(t, resultKinds, expectedKinds, "Error in expected destKinds in Test_SearchResolver_Relationships")

	// Verify returned items.
	if len(result) != len(mockRows.mockData) {
		t.Errorf("Items() received incorrect number of items. Expected %d Got: %d", len(mockRows.mockData), len(result))
	}

}

func Test_SearchResolver_RelationshipsWithCluster(t *testing.T) {
	config.Cfg.RelationLevel = 3
	var resultList []*string

	uid1 := "cluster__local-cluster"
	csRes, nsRes, mc := newUserData()
	ud := rbac.UserData{CsResources: csRes, NsResources: nsRes, ManagedClusters: mc}
	resultList = append(resultList, &uid1)

	// take the uids from above as input
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "kind", Values: resultList}}}
	resolver, mockPool2 := newMockSearchResolver(t, searchInput, resultList, &ud, nil)

	relQuery := strings.TrimSpace(`SELECT "related"."uid", "related"."kind", "related"."level" FROM (SELECT "uid", "kind", MIN("level") AS "level" FROM (SELECT "level", unnest(array[sourceid, destid, concat('cluster__',cluster)]) AS "uid", unnest(array[sourcekind, destkind, 'Cluster']) AS "kind" FROM (WITH RECURSIVE search_graph(level, sourceid, destid,  sourcekind, destkind, cluster) AS (SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster" FROM "search"."edges" AS "e" WHERE (("destid" IN ('cluster__local-cluster')) OR ("sourceid" IN ('cluster__local-cluster'))) UNION (SELECT level+1 AS "level", "e"."sourceid", "e"."destid", "e"."sourcekind", "e"."destkind", "e"."cluster" FROM "search"."edges" AS "e" INNER JOIN "search_graph" AS "sg" ON (("sg"."destid" IN ("e"."sourceid", "e"."destid")) OR ("sg"."sourceid" IN ("e"."sourceid", "e"."destid"))) WHERE (("e"."destkind" NOT IN ('Node', 'Channel')) AND ("e"."sourcekind" NOT IN ('Node', 'Channel')) AND ("sg"."level" <= 3)))) SELECT DISTINCT "level", "sourceid", "destid", "sourcekind", "destkind", "cluster" FROM "search_graph") AS "search_graph") AS "combineIds" WHERE (("level" <= 3) AND ("uid" NOT IN ('cluster__local-cluster'))) GROUP BY "uid", "kind" UNION (SELECT "uid" AS "uid", data->>'kind' AS "kind", 1 AS "level" FROM "search"."resources" WHERE ("cluster" IN ('local-cluster')))) AS "related" INNER JOIN "search"."resources" ON ("related"."uid" = "resources".uid) WHERE (("cluster" = ANY ('{"managed1","managed2"}')) OR ((data->>'_hubClusterResource' = 'true') AND (((COALESCE(data->>'namespace', '') = '') AND (((COALESCE(data->>'apigroup', '') = '') AND (data->>'kind_plural' = 'nodes')) OR ((COALESCE(data->>'apigroup', '') = 'storage.k8s.io') AND (data->>'kind_plural' = 'csinodes')))) OR (((data->>'namespace' = 'default') AND (((COALESCE(data->>'apigroup', '') = '') AND (data->>'kind_plural' = 'configmaps')) OR ((COALESCE(data->>'apigroup', '') = 'v4') AND (data->>'kind_plural' = 'services')))) OR ((data->>'namespace' = 'ocm') AND (((COALESCE(data->>'apigroup', '') = 'v1') AND (data->>'kind_plural' = 'pods')) OR ((COALESCE(data->>'apigroup', '') = 'v2') AND (data->>'kind_plural' = 'deployments'))))))))`)

	mockRows := newMockRowsWithoutRBAC("./mocks/mock-rel-1.json", searchInput, "", 0)
	mockPool2.EXPECT().Query(gomock.Any(),
		gomock.Eq(relQuery),
		gomock.Eq([]interface{}{}),
	).Return(mockRows, nil)

	result := resolver.Related(context.Background()) // this should return a relatedResults object

	resultKinds := make([]*string, len(result))
	for i, data := range result {
		kind := data.Kind
		resultKinds[i] = &kind
	}

	expectedKinds := make([]*string, len(mockRows.mockData))
	for i, data := range mockRows.mockData {
		destKind, _ := data["kind"].(string)
		expectedKinds[i] = &destKind
	}
	// Verify expected and result kinds
	AssertStringArrayEqual(t, resultKinds, expectedKinds, "Error in expected destKinds in Test_SearchResolver_Relationships")

	// Verify returned items.
	if len(result) != len(mockRows.mockData) {
		t.Errorf("Items() received incorrect number of items. Expected %d Got: %d", len(mockRows.mockData), len(result))
	}

}
func Test_SearchResolver_RelatedKindsRelationships(t *testing.T) {
	config.Cfg.RelationLevel = 3

	var resultList []*string
	uid1 := "local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd"
	uid2 := "local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b"

	resultList = append(resultList, &uid1, &uid2)
	relatedKind1 := "ConfigMap"
	csRes, nsRes, mc := newUserData()
	ud := rbac.UserData{CsResources: csRes, NsResources: nsRes, ManagedClusters: mc}
	// take the uids from above as input
	searchInput2 := &model.SearchInput{RelatedKinds: []*string{&relatedKind1}, Filters: []*model.SearchFilter{{Property: "uid", Values: resultList}}}

	resolver, mockPool2 := newMockSearchResolver(t, searchInput2, resultList, &ud, nil)

	relQuery := strings.TrimSpace(`SELECT "related"."uid", "related"."kind", "related"."level" FROM (SELECT "uid", "kind", MIN("level") AS "level" FROM (SELECT "level", unnest(array[sourceid, destid, concat('cluster__',cluster)]) AS "uid", unnest(array[sourcekind, destkind, 'Cluster']) AS "kind" FROM (WITH RECURSIVE search_graph(level, sourceid, destid,  sourcekind, destkind, cluster) AS (SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster" FROM "search"."edges" AS "e" WHERE (("destid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b')) OR ("sourceid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b'))) UNION (SELECT level+1 AS "level", "e"."sourceid", "e"."destid", "e"."sourcekind", "e"."destkind", "e"."cluster" FROM "search"."edges" AS "e" INNER JOIN "search_graph" AS "sg" ON (("sg"."destid" IN ("e"."sourceid", "e"."destid")) OR ("sg"."sourceid" IN ("e"."sourceid", "e"."destid"))) WHERE (("e"."destkind" NOT IN ('Node', 'Channel')) AND ("e"."sourcekind" NOT IN ('Node', 'Channel')) AND ("sg"."level" <= 3)))) SELECT DISTINCT "level", "sourceid", "destid", "sourcekind", "destkind", "cluster" FROM "search_graph") AS "search_graph") AS "combineIds" WHERE (("level" <= 3) AND ("uid" NOT IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b'))) GROUP BY "uid", "kind") AS "related" INNER JOIN "search"."resources" ON ("related"."uid" = "resources".uid) WHERE (("cluster" = ANY ('{"managed1","managed2"}')) OR ((data->>'_hubClusterResource' = 'true') AND (((COALESCE(data->>'namespace', '') = '') AND (((COALESCE(data->>'apigroup', '') = '') AND (data->>'kind_plural' = 'nodes')) OR ((COALESCE(data->>'apigroup', '') = 'storage.k8s.io') AND (data->>'kind_plural' = 'csinodes')))) OR (((data->>'namespace' = 'default') AND (((COALESCE(data->>'apigroup', '') = '') AND (data->>'kind_plural' = 'configmaps')) OR ((COALESCE(data->>'apigroup', '') = 'v4') AND (data->>'kind_plural' = 'services')))) OR ((data->>'namespace' = 'ocm') AND (((COALESCE(data->>'apigroup', '') = 'v1') AND (data->>'kind_plural' = 'pods')) OR ((COALESCE(data->>'apigroup', '') = 'v2') AND (data->>'kind_plural' = 'deployments'))))))))`)

	mockRows := newMockRowsWithoutRBAC("./mocks/mock-rel-1.json", searchInput2, "", 0)
	mockPool2.EXPECT().Query(gomock.Any(),
		gomock.Eq(relQuery),
		gomock.Eq([]interface{}{}),
	).Return(mockRows, nil)
	mockRows2 := newMockRowsWithoutRBAC("./mocks/mock.json", searchInput2, "", 0)

	relatedQuery := `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("uid" IN ('local-cluster/30c35f12-320a-417f-98d1-fbee28a4b2a6')) LIMIT 1000`
	// Mock the database query
	mockPool2.EXPECT().Query(gomock.Any(),
		gomock.Eq(relatedQuery),
		gomock.Eq([]interface{}{}),
	).Return(mockRows2, nil)

	result := resolver.Related(context.Background()) // this should return a relatedResults object

	if !strings.EqualFold(result[0].Kind, strings.ToLower(mockRows2.mockData[0]["destkind"].(string))) {
		t.Errorf("Kind value in mockdata does not match kind value of result")
	}

	// Verify returned items.
	if len(result) != len(mockRows2.mockData) {
		t.Errorf("Items() received incorrect number of items. Expected %d Got: %d", len(mockRows.mockData), len(result))
	}
}

func Test_SearchResolver_RelatedKindsRelationships_NegativeLimit(t *testing.T) {
	config.Cfg.RelationLevel = 3
	var resultList []*string

	uid1 := "local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd"
	uid2 := "local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b"

	limit := -1
	resultList = append(resultList, &uid1, &uid2)
	relatedKind1 := "ConfigMap"
	// take the uids from above as input
	searchInput2 := &model.SearchInput{Limit: &limit, RelatedKinds: []*string{&relatedKind1}, Filters: []*model.SearchFilter{{Property: "uid", Values: resultList}}}
	ud := rbac.UserData{}
	resolver, mockPool2 := newMockSearchResolver(t, searchInput2, resultList, &ud, nil)

	relQuery := strings.TrimSpace(`SELECT "related"."uid", "related"."kind", "related"."level" FROM (SELECT "uid", "kind", MIN("level") AS "level" FROM (SELECT "level", unnest(array[sourceid, destid, concat('cluster__',cluster)]) AS "uid", unnest(array[sourcekind, destkind, 'Cluster']) AS "kind" FROM (WITH RECURSIVE search_graph(level, sourceid, destid,  sourcekind, destkind, cluster) AS (SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster" FROM "search"."edges" AS "e" WHERE (("destid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b')) OR ("sourceid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b'))) UNION (SELECT level+1 AS "level", "e"."sourceid", "e"."destid", "e"."sourcekind", "e"."destkind", "e"."cluster" FROM "search"."edges" AS "e" INNER JOIN "search_graph" AS "sg" ON (("sg"."destid" IN ("e"."sourceid", "e"."destid")) OR ("sg"."sourceid" IN ("e"."sourceid", "e"."destid"))) WHERE (("e"."destkind" NOT IN ('Node', 'Channel')) AND ("e"."sourcekind" NOT IN ('Node', 'Channel')) AND ("sg"."level" <= 3)))) SELECT DISTINCT "level", "sourceid", "destid", "sourcekind", "destkind", "cluster" FROM "search_graph") AS "search_graph") AS "combineIds" WHERE (("level" <= 3) AND ("uid" NOT IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b'))) GROUP BY "uid", "kind") AS "related" INNER JOIN "search"."resources" ON ("related"."uid" = "resources".uid) WHERE (("cluster" = ANY ('{}')) OR ((data->>'_hubClusterResource' = 'true') AND NULL))`)

	mockRows := newMockRowsWithoutRBAC("./mocks/mock-rel-1.json", searchInput2, "", 0)
	mockPool2.EXPECT().Query(gomock.Any(),
		gomock.Eq(relQuery),
		gomock.Eq([]interface{}{}),
	).Return(mockRows, nil)
	mockRows2 := newMockRowsWithoutRBAC("./mocks/mock.json", searchInput2, "", 0)

	relatedQuery := `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("uid" IN ('local-cluster/30c35f12-320a-417f-98d1-fbee28a4b2a6'))`
	// Mock the database query
	mockPool2.EXPECT().Query(gomock.Any(),
		gomock.Eq(relatedQuery),
		gomock.Eq([]interface{}{}),
	).Return(mockRows2, nil)

	result := resolver.Related(context.Background()) // this should return a relatedResults object

	if !strings.EqualFold(result[0].Kind, strings.ToLower(mockRows2.mockData[0]["destkind"].(string))) {
		t.Errorf("Kind value in mockdata does not match kind value of result")
	}

	// Verify returned items.
	if len(result) != len(mockRows2.mockData) {
		t.Errorf("Items() received incorrect number of items. Expected %d Got: %d", len(mockRows.mockData), len(result))
	}
}

func Test_SearchResolver_Level1Related(t *testing.T) {
	config.Cfg.RelationLevel = 0
	var resultList []*string
	uid1 := "local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd"
	uid2 := "local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b"

	limit := -1
	resultList = append(resultList, &uid1, &uid2)
	relatedKind1 := "ConfigMap"
	// take the uids from above as input
	searchInput2 := &model.SearchInput{Limit: &limit, RelatedKinds: []*string{&relatedKind1}, Filters: []*model.SearchFilter{{Property: "uid", Values: resultList}}}
	csRes, nsRes, mc := newUserData()
	ud := rbac.UserData{CsResources: csRes, NsResources: nsRes, ManagedClusters: mc}
	resolver, mockPool2 := newMockSearchResolver(t, searchInput2, resultList, &ud, nil)

	relQuery := strings.TrimSpace(`SELECT "related"."uid", "related"."kind", "related"."level" FROM (SELECT "uid", "kind", MIN("level") AS "level" FROM (SELECT "level", unnest(array[sourceid, destid, concat('cluster__',cluster)]) AS "uid", unnest(array[sourcekind, destkind, 'Cluster']) AS "kind" FROM (SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster" FROM "search"."edges" AS "e" WHERE (("destid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b')) OR ("sourceid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b')))) AS "search_graph") AS "combineIds" WHERE (("level" <= 1) AND ("uid" NOT IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b'))) GROUP BY "uid", "kind") AS "related" INNER JOIN "search"."resources" ON ("related"."uid" = "resources".uid) WHERE (("cluster" = ANY ('{"managed1","managed2"}')) OR ((data->>'_hubClusterResource' = 'true') AND (((COALESCE(data->>'namespace', '') = '') AND (((COALESCE(data->>'apigroup', '') = '') AND (data->>'kind_plural' = 'nodes')) OR ((COALESCE(data->>'apigroup', '') = 'storage.k8s.io') AND (data->>'kind_plural' = 'csinodes')))) OR (((data->>'namespace' = 'default') AND (((COALESCE(data->>'apigroup', '') = '') AND (data->>'kind_plural' = 'configmaps')) OR ((COALESCE(data->>'apigroup', '') = 'v4') AND (data->>'kind_plural' = 'services')))) OR ((data->>'namespace' = 'ocm') AND (((COALESCE(data->>'apigroup', '') = 'v1') AND (data->>'kind_plural' = 'pods')) OR ((COALESCE(data->>'apigroup', '') = 'v2') AND (data->>'kind_plural' = 'deployments'))))))))`)

	mockRows := newMockRowsWithoutRBAC("./mocks/mock-rel-1.json", searchInput2, "", 0)
	mockPool2.EXPECT().Query(gomock.Any(),
		gomock.Eq(relQuery),
		gomock.Eq([]interface{}{}),
	).Return(mockRows, nil)
	mockRows2 := newMockRowsWithoutRBAC("./mocks/mock.json", searchInput2, "", 0)

	relatedQuery := `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("uid" IN ('local-cluster/30c35f12-320a-417f-98d1-fbee28a4b2a6'))`
	// Mock the database query
	mockPool2.EXPECT().Query(gomock.Any(),
		gomock.Eq(relatedQuery),
		gomock.Eq([]interface{}{}),
	).Return(mockRows2, nil)

	result := resolver.Related(context.Background()) // this should return a relatedResults object

	if !strings.EqualFold(result[0].Kind, strings.ToLower(mockRows2.mockData[0]["destkind"].(string))) {
		t.Errorf("Kind value in mockdata does not match kind value of result")
	}

	// Verify returned items.
	if len(result) != len(mockRows2.mockData) {
		t.Errorf("Items() received incorrect number of items. Expected %d Got: %d", len(mockRows.mockData), len(result))
	}
}
