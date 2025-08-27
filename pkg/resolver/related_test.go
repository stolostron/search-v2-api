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
	"github.com/stretchr/testify/assert"
)

func Test_SearchResolver_Relationships(t *testing.T) {
	config.Cfg.RelationLevel = 3

	// Build a mock SearchResolver{} using uids as filter input.
	uid1 := "local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd"
	uid2 := "local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b"
	resultList := []*string{&uid1, &uid2}
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "uid", Values: resultList}}}
	resolver, mockPool := newMockSearchResolver(t, searchInput, resultList, rbac.UserData{CsResources: []rbac.Resource{}}, nil)

	// Mock FIRST database request.
	query := strings.TrimSpace(`SELECT "related"."uid", "related"."kind", "related"."level", "related"."path" FROM (SELECT "uid", "kind", MIN("level") AS "level", "path" FROM (SELECT "level", unnest(array[sourceid, destid, concat('cluster__',cluster)]) AS "uid", unnest(array[sourcekind, destkind, 'Cluster']) AS "kind", "path" FROM (WITH RECURSIVE search_graph(level, sourceid, destid,  sourcekind, destkind, cluster, path) AS (SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster", array[sourceid, destid] AS "path" FROM "search"."edges" AS "e" WHERE ("sourceid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b')) UNION ALL (SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster", array[sourceid, destid] AS "path" FROM "search"."edges" AS "e" WHERE ("sourceid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b'))) UNION ALL (SELECT level+1 AS "level", "e"."sourceid", "e"."destid", "e"."sourcekind", "e"."destkind", "e"."cluster", "path" FROM "search"."edges" AS "e" INNER JOIN "search_graph" AS "sg" ON ("sg"."sourceid" IN ("e"."sourceid", "e"."destid")) WHERE (("e"."destkind" NOT IN ('Node', 'Channel')) AND ("e"."sourcekind" NOT IN ('Node', 'Channel')) AND ("sg"."level" <= 3)) UNION ALL (SELECT level+1 AS "level", "e"."sourceid", "e"."destid", "e"."sourcekind", "e"."destkind", "e"."cluster", "path" FROM "search"."edges" AS "e" INNER JOIN "search_graph" AS "sg" ON ("sg"."destid" IN ("e"."sourceid", "e"."destid")) WHERE (("e"."destkind" NOT IN ('Node', 'Channel')) AND ("e"."sourcekind" NOT IN ('Node', 'Channel')) AND ("sg"."level" <= 3))))) SELECT DISTINCT "level", "sourceid", "destid", "sourcekind", "destkind", "cluster", "path" FROM "search_graph") AS "search_graph") AS "combineIds" WHERE (("level" <= 3) AND ("uid" NOT IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b'))) GROUP BY "uid", "kind", "path") AS "related" INNER JOIN "search"."resources" ON ("related"."uid" = "resources".uid) WHERE ("cluster" = ANY ('{}'))`)
	mockRows := newMockRowsWithoutRBAC("./mocks/mock-rel-1.json", searchInput, "", 0)
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(query),
		gomock.Any(),
	).Return(mockRows, nil)

	// Mock SECOND database request.
	query2 := `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("uid" IN ('local-cluster/411e30e4-f773-41a6-b745-24c93c173f45', 'local-cluster/30c35f12-320a-417f-98d1-fbee28a4b2a6')) LIMIT 1000`
	mockRows2 := newMockRows("./mocks/mock-related-test.json")
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(query2),
		gomock.Any(),
	).Return(mockRows2, nil)

	// Execute the function - should return a relatedResults object
	result, err := resolver.Related(context.Background())
	assert.Nil(t, err)
	// Verify expected and result kinds
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

	AssertStringArrayEqual(t, resultKinds, expectedKinds, "Error in expected destKinds in Test_SearchResolver_Relationships")

	// Verify returned items.
	if len(result) != len(mockRows.mockData) {
		t.Errorf("Items() received incorrect number of items. Expected %d Got: %d", len(mockRows.mockData), len(result))
	}
}

// FIXME: Intermittent failure.
func Test_SearchResolver_RelationshipsWithCluster(t *testing.T) {
	config.Cfg.RelationLevel = 3

	// Build a mock SearchResolver{} using uids as filter input.
	uid1 := "cluster__local-cluster"
	resultList := []*string{&uid1}
	csRes, nsRes, mc := newUserData()
	ud := rbac.UserData{CsResources: csRes, NsResources: nsRes, ManagedClusters: mc}
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "kind", Values: resultList}}}
	resolver, mockPool := newMockSearchResolver(t, searchInput, resultList, ud, nil)

	// Mock FIRST database request.
	query1 := strings.TrimSpace(`SELECT "related"."uid", "related"."kind", "related"."level", "related"."path" FROM (SELECT "uid", "kind", MIN("level") AS "level", "path" FROM (SELECT "level", unnest(array[sourceid, destid, concat('cluster__',cluster)]) AS "uid", unnest(array[sourcekind, destkind, 'Cluster']) AS "kind", "path" FROM (WITH RECURSIVE search_graph(level, sourceid, destid,  sourcekind, destkind, cluster, path) AS (SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster", array[sourceid, destid] AS "path" FROM "search"."edges" AS "e" WHERE ("sourceid" IN ('cluster__local-cluster')) UNION ALL (SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster", array[sourceid, destid] AS "path" FROM "search"."edges" AS "e" WHERE ("sourceid" IN ('cluster__local-cluster'))) UNION ALL (SELECT level+1 AS "level", "e"."sourceid", "e"."destid", "e"."sourcekind", "e"."destkind", "e"."cluster", "path" FROM "search"."edges" AS "e" INNER JOIN "search_graph" AS "sg" ON ("sg"."sourceid" IN ("e"."sourceid", "e"."destid")) WHERE (("e"."destkind" NOT IN ('Node', 'Channel')) AND ("e"."sourcekind" NOT IN ('Node', 'Channel')) AND ("sg"."level" <= 3)) UNION ALL (SELECT level+1 AS "level", "e"."sourceid", "e"."destid", "e"."sourcekind", "e"."destkind", "e"."cluster", "path" FROM "search"."edges" AS "e" INNER JOIN "search_graph" AS "sg" ON ("sg"."destid" IN ("e"."sourceid", "e"."destid")) WHERE (("e"."destkind" NOT IN ('Node', 'Channel')) AND ("e"."sourcekind" NOT IN ('Node', 'Channel')) AND ("sg"."level" <= 3))))) SELECT DISTINCT "level", "sourceid", "destid", "sourcekind", "destkind", "cluster", "path" FROM "search_graph") AS "search_graph") AS "combineIds" WHERE (("level" <= 3) AND ("uid" NOT IN ('cluster__local-cluster'))) GROUP BY "uid", "kind", "path" UNION (SELECT "uid" AS "uid", data->>'kind' AS "kind", 1 AS "level", array[]::text[] AS "path" FROM "search"."resources" WHERE ("cluster" IN ('local-cluster')))) AS "related" INNER JOIN "search"."resources" ON ("related"."uid" = "resources".uid) WHERE (("cluster" = ANY ('{"managed1","managed2"}')) OR ("data"?'_hubClusterResource' AND ((NOT("data"?'namespace') AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'nodes') OR (data->'apigroup'?'storage.k8s.io' AND data->'kind_plural'?'csinodes'))) OR ((data->'namespace'?|'{"default"}' AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'configmaps') OR (data->'apigroup'?'v4' AND data->'kind_plural'?'services'))) OR (data->'namespace'?|'{"ocm"}' AND ((data->'apigroup'?'v1' AND data->'kind_plural'?'pods') OR (data->'apigroup'?'v2' AND data->'kind_plural'?'deployments')))))))`)
	mockRows := newMockRowsWithoutRBAC("./mocks/mock-rel-1.json", searchInput, "", 0)
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(query1),
		gomock.Eq([]interface{}{}),
	).Return(mockRows, nil)

	// Mock the SECOND database request.
	query2 := `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("uid" IN ('local-cluster/411e30e4-f773-41a6-b745-24c93c173f45', 'local-cluster/30c35f12-320a-417f-98d1-fbee28a4b2a6')) LIMIT 1000`
	mockRows2 := newMockRows("./mocks/mock-related-test.json")
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(query2),
		gomock.Any(),
	).Return(mockRows2, nil)

	// Execute the function - should return a relatedResults object
	result, err := resolver.Related(context.Background())
	assert.Nil(t, err)

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

	// Build a mock SearchResolver{} using uids as filter input.
	uid1 := "local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd"
	uid2 := "local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b"
	resultList := []*string{&uid1, &uid2}
	relatedKind := "ConfigMap"
	csRes, nsRes, mc := newUserData()
	ud := rbac.UserData{CsResources: csRes, NsResources: nsRes, ManagedClusters: mc}
	searchInput := &model.SearchInput{RelatedKinds: []*string{&relatedKind}, Filters: []*model.SearchFilter{{Property: "uid", Values: resultList}}}
	resolver, mockPool := newMockSearchResolver(t, searchInput, resultList, ud, nil)

	// Mock the FIRST database request.
	query1 := strings.TrimSpace(`SELECT "related"."uid", "related"."kind", "related"."level", "related"."path" FROM (SELECT "uid", "kind", MIN("level") AS "level", "path" FROM (SELECT "level", unnest(array[sourceid, destid, concat('cluster__',cluster)]) AS "uid", unnest(array[sourcekind, destkind, 'Cluster']) AS "kind", "path" FROM (WITH RECURSIVE search_graph(level, sourceid, destid,  sourcekind, destkind, cluster, path) AS (SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster", array[sourceid, destid] AS "path" FROM "search"."edges" AS "e" WHERE ("sourceid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b')) UNION ALL (SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster", array[sourceid, destid] AS "path" FROM "search"."edges" AS "e" WHERE ("sourceid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b'))) UNION ALL (SELECT level+1 AS "level", "e"."sourceid", "e"."destid", "e"."sourcekind", "e"."destkind", "e"."cluster", "path" FROM "search"."edges" AS "e" INNER JOIN "search_graph" AS "sg" ON ("sg"."sourceid" IN ("e"."sourceid", "e"."destid")) WHERE (("e"."destkind" NOT IN ('Node', 'Channel')) AND ("e"."sourcekind" NOT IN ('Node', 'Channel')) AND ("sg"."level" <= 3)) UNION ALL (SELECT level+1 AS "level", "e"."sourceid", "e"."destid", "e"."sourcekind", "e"."destkind", "e"."cluster", "path" FROM "search"."edges" AS "e" INNER JOIN "search_graph" AS "sg" ON ("sg"."destid" IN ("e"."sourceid", "e"."destid")) WHERE (("e"."destkind" NOT IN ('Node', 'Channel')) AND ("e"."sourcekind" NOT IN ('Node', 'Channel')) AND ("sg"."level" <= 3))))) SELECT DISTINCT "level", "sourceid", "destid", "sourcekind", "destkind", "cluster", "path" FROM "search_graph") AS "search_graph") AS "combineIds" WHERE (("level" <= 3) AND ("uid" NOT IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b'))) GROUP BY "uid", "kind", "path") AS "related" INNER JOIN "search"."resources" ON ("related"."uid" = "resources".uid) WHERE (("cluster" = ANY ('{"managed1","managed2"}')) OR ("data"?'_hubClusterResource' AND ((NOT("data"?'namespace') AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'nodes') OR (data->'apigroup'?'storage.k8s.io' AND data->'kind_plural'?'csinodes'))) OR ((data->'namespace'?|'{"default"}' AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'configmaps') OR (data->'apigroup'?'v4' AND data->'kind_plural'?'services'))) OR (data->'namespace'?|'{"ocm"}' AND ((data->'apigroup'?'v1' AND data->'kind_plural'?'pods') OR (data->'apigroup'?'v2' AND data->'kind_plural'?'deployments')))))))`)
	mockRows := newMockRowsWithoutRBAC("./mocks/mock-rel-1.json", searchInput, "", 0)
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(query1),
		gomock.Eq([]interface{}{}),
	).Return(mockRows, nil)

	// Mock the SECOND database request.
	query2 := `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("uid" IN ('local-cluster/30c35f12-320a-417f-98d1-fbee28a4b2a6')) LIMIT 1000`
	mockRows2 := newMockRowsWithoutRBAC("./mocks/mock.json", searchInput, "", 0)
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(query2),
		gomock.Eq([]interface{}{}),
	).Return(mockRows2, nil)

	// Execute the function - should return a relatedResults object
	result, err := resolver.Related(context.Background())
	assert.Nil(t, err)

	// Verify returned items.
	if !strings.EqualFold(result[0].Kind, strings.ToLower(mockRows2.mockData[0]["destkind"].(string))) {
		t.Errorf("Kind value in mockdata does not match kind value of result")
	}
	if len(result) != len(mockRows2.mockData) {
		t.Errorf("Items() received incorrect number of items. Expected %d Got: %d", len(mockRows.mockData), len(result))
	}
}

func Test_SearchResolver_RelatedKindsRelationships_NegativeLimit(t *testing.T) {
	config.Cfg.RelationLevel = 3

	// Build a mock SearchResolver{} using uids as filter input.
	uid1 := "local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd"
	uid2 := "local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b"
	resultList := []*string{&uid1, &uid2}
	limit := -1
	relatedKind := "ConfigMap"
	searchInput := &model.SearchInput{Limit: &limit, RelatedKinds: []*string{&relatedKind}, Filters: []*model.SearchFilter{{Property: "uid", Values: resultList}}}
	resolver, mockPool := newMockSearchResolver(t, searchInput, resultList, rbac.UserData{CsResources: []rbac.Resource{}}, nil)

	// Mock the FIRST database request.
	query := strings.TrimSpace(`SELECT "related"."uid", "related"."kind", "related"."level", "related"."path" FROM (SELECT "uid", "kind", MIN("level") AS "level", "path" FROM (SELECT "level", unnest(array[sourceid, destid, concat('cluster__',cluster)]) AS "uid", unnest(array[sourcekind, destkind, 'Cluster']) AS "kind", "path" FROM (WITH RECURSIVE search_graph(level, sourceid, destid,  sourcekind, destkind, cluster, path) AS (SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster", array[sourceid, destid] AS "path" FROM "search"."edges" AS "e" WHERE ("sourceid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b')) UNION ALL (SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster", array[sourceid, destid] AS "path" FROM "search"."edges" AS "e" WHERE ("sourceid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b'))) UNION ALL (SELECT level+1 AS "level", "e"."sourceid", "e"."destid", "e"."sourcekind", "e"."destkind", "e"."cluster", "path" FROM "search"."edges" AS "e" INNER JOIN "search_graph" AS "sg" ON ("sg"."sourceid" IN ("e"."sourceid", "e"."destid")) WHERE (("e"."destkind" NOT IN ('Node', 'Channel')) AND ("e"."sourcekind" NOT IN ('Node', 'Channel')) AND ("sg"."level" <= 3)) UNION ALL (SELECT level+1 AS "level", "e"."sourceid", "e"."destid", "e"."sourcekind", "e"."destkind", "e"."cluster", "path" FROM "search"."edges" AS "e" INNER JOIN "search_graph" AS "sg" ON ("sg"."destid" IN ("e"."sourceid", "e"."destid")) WHERE (("e"."destkind" NOT IN ('Node', 'Channel')) AND ("e"."sourcekind" NOT IN ('Node', 'Channel')) AND ("sg"."level" <= 3))))) SELECT DISTINCT "level", "sourceid", "destid", "sourcekind", "destkind", "cluster", "path" FROM "search_graph") AS "search_graph") AS "combineIds" WHERE (("level" <= 3) AND ("uid" NOT IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b'))) GROUP BY "uid", "kind", "path") AS "related" INNER JOIN "search"."resources" ON ("related"."uid" = "resources".uid) WHERE ("cluster" = ANY ('{}'))`)
	mockRows := newMockRowsWithoutRBAC("./mocks/mock-rel-1.json", searchInput, "", 0)
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(query),
		gomock.Eq([]interface{}{}),
	).Return(mockRows, nil)

	// Mock the SECOND database request.
	query2 := `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("uid" IN ('local-cluster/30c35f12-320a-417f-98d1-fbee28a4b2a6'))`
	mockRows2 := newMockRowsWithoutRBAC("./mocks/mock.json", searchInput, "", 0)
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(query2),
		gomock.Eq([]interface{}{}),
	).Return(mockRows2, nil)

	// Execute the function - should return a relatedResults object
	result, err := resolver.Related(context.Background())
	assert.Nil(t, err)

	// Verify returned items.
	if !strings.EqualFold(result[0].Kind, strings.ToLower(mockRows2.mockData[0]["destkind"].(string))) {
		t.Errorf("Kind value in mockdata does not match kind value of result")
	}
	if len(result) != len(mockRows2.mockData) {
		t.Errorf("Items() received incorrect number of items. Expected %d Got: %d", len(mockRows.mockData), len(result))
	}
}

func Test_SearchResolver_Level1Related(t *testing.T) {
	config.Cfg.RelationLevel = 0

	// Build a mock SearchResolver{} using uids as filter input.
	uid1 := "local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd"
	uid2 := "local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b"
	resultList := []*string{&uid1, &uid2}
	limit := -1
	relatedKind := "ConfigMap"
	searchInput := &model.SearchInput{Limit: &limit, RelatedKinds: []*string{&relatedKind}, Filters: []*model.SearchFilter{{Property: "uid", Values: resultList}}}
	csRes, nsRes, mc := newUserData()
	ud := rbac.UserData{CsResources: csRes, NsResources: nsRes, ManagedClusters: mc}
	resolver, mockPool := newMockSearchResolver(t, searchInput, resultList, ud, nil)

	// Mock the FIRST database request.
	query := strings.TrimSpace(`SELECT "related"."uid", "related"."kind", "related"."level", "related"."path" FROM (SELECT "uid", "kind", MIN("level") AS "level", "path" FROM (SELECT "level", unnest(array[sourceid, destid, concat('cluster__',cluster)]) AS "uid", unnest(array[sourcekind, destkind, 'Cluster']) AS "kind", "path" FROM (SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster", array[sourceid, destid] AS "path" FROM "search"."edges" AS "e" WHERE ("sourceid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b')) UNION ALL (SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster", array[sourceid, destid] AS "path" FROM "search"."edges" AS "e" WHERE ("sourceid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b')))) AS "search_graph") AS "combineIds" WHERE (("level" <= 1) AND ("uid" NOT IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b'))) GROUP BY "uid", "kind", "path") AS "related" INNER JOIN "search"."resources" ON ("related"."uid" = "resources".uid) WHERE (("cluster" = ANY ('{"managed1","managed2"}')) OR ("data"?'_hubClusterResource' AND ((NOT("data"?'namespace') AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'nodes') OR (data->'apigroup'?'storage.k8s.io' AND data->'kind_plural'?'csinodes'))) OR ((data->'namespace'?|'{"default"}' AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'configmaps') OR (data->'apigroup'?'v4' AND data->'kind_plural'?'services'))) OR (data->'namespace'?|'{"ocm"}' AND ((data->'apigroup'?'v1' AND data->'kind_plural'?'pods') OR (data->'apigroup'?'v2' AND data->'kind_plural'?'deployments')))))))`)
	mockRows := newMockRowsWithoutRBAC("./mocks/mock-rel-1.json", searchInput, "", 0)
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(query),
		gomock.Eq([]interface{}{}),
	).Return(mockRows, nil)

	// Mock the SECOND database request.
	query2 := `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("uid" IN ('local-cluster/30c35f12-320a-417f-98d1-fbee28a4b2a6'))`
	mockRows2 := newMockRowsWithoutRBAC("./mocks/mock.json", searchInput, "", 0)
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(query2),
		gomock.Eq([]interface{}{}),
	).Return(mockRows2, nil)

	// Execute the function - should return a relatedResults object
	result, err := resolver.Related(context.Background())
	assert.Nil(t, err)

	// Verify returned items.
	if !strings.EqualFold(result[0].Kind, strings.ToLower(mockRows2.mockData[0]["destkind"].(string))) {
		t.Errorf("Kind value in mockdata does not match kind value of result")
	}
	if len(result) != len(mockRows2.mockData) {
		t.Errorf("Items() received incorrect number of items. Expected %d Got: %d", len(mockRows.mockData), len(result))
	}
}

func Test_SearchResolver_Relationships_NoUserData(t *testing.T) {
	config.Cfg.RelationLevel = 3

	// Build a mock SearchResolver{} using uids as filter input.
	uid1 := "local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd"
	uid2 := "local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b"
	resultList := []*string{&uid1, &uid2}
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "uid", Values: resultList}}}
	resolver, mockPool := newMockSearchResolver(t, searchInput, resultList, rbac.UserData{}, nil)

	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(``), //query will be empty as user data for rbac is not provided
		gomock.Eq([]interface{}{}),
	).Return(nil, nil)
	// This should become empty after function execution
	resolver.query = "mock Query"

	// Execute the function - should return a relatedResults object
	_, err := resolver.Related(context.Background())
	assert.Nil(t, err)
	// Verify expected and result kinds
	assert.Equal(t, resolver.query, "", "query should be empty as user data is not provided")

}

func TestCheckIfInArray(t *testing.T) {
	shouldbeTrue := CheckIfInArray([]string{"uid123", "uid234"}, "uid123")
	shouldbeFalse := CheckIfInArray([]string{"uid123", "uid234"}, "uid456")
	// Verify expected result
	assert.True(t, shouldbeTrue, "uid uid123 is present in the list. Expected true, got %t", shouldbeTrue)
	// Verify expected result
	assert.Falsef(t, shouldbeFalse, "uid uid456 is not present in the list. Expected false, got %t", shouldbeFalse)
}

func TestUpdResultToCurrSearchUidsMap(t *testing.T) {
	// Build a mock SearchResolver{} using uids as filter input.
	uid1 := "local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd"
	uid2 := "local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b"
	resultList := []*string{&uid1, &uid2}
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "uid", Values: resultList}}}
	resolver, _ := newMockSearchResolver(t, searchInput, resultList, rbac.UserData{}, nil)
	resultMap := map[string][]string{}
	//uid123 should get newly mapped to uid234
	resolver.updResultToCurrSearchUidsMap("uid123", map[string]struct{}{"uid234": {}, "uid345": {}},
		resultMap, []string{"uid234", "uid123"})
	assert.Equal(t, len(resultMap), 1, "There should be one related uid in the map")
	assert.Equal(t, resultMap["uid123"], []string{"uid234"}, "There should be one related uid in the map")

	//uid123 should get newly mapped to uid567
	resolver.updResultToCurrSearchUidsMap("uid123", map[string]struct{}{"uid567": {}, "uid678": {}},
		resultMap, []string{"uid123", "uid567"})
	assert.Equal(t, len(resultMap["uid123"]), 2, "There should be two related uids in the map")
	assert.Equal(t, resultMap["uid123"], []string{"uid234", "uid567"}, "There should be 2 related uids in the map")

	//uid123 should get newly mapped to uid567
	resolver.updResultToCurrSearchUidsMap("uid678", map[string]struct{}{"uid567": {}, "uid789": {}},
		resultMap, []string{"uid567", "uid678"})
	assert.Equal(t, len(resultMap), 2, "There should be two uids in the map")
	assert.Equal(t, len(resultMap["uid678"]), 1, "There should be two related uids in the map")
	assert.Equal(t, resultMap["uid678"], []string{"uid567"}, "There should be 1 related uid in the map")

	//resultMap shouldn't get changed as uid123 is already mapped to uid567
	resolver.updResultToCurrSearchUidsMap("uid678", map[string]struct{}{"uid567": {}, "uid789": {}},
		resultMap, []string{"uid567", "uid678"})
	assert.Equal(t, len(resultMap), 2, "There should be two uids in the map")
	assert.Equal(t, len(resultMap["uid678"]), 1, "There should be two related uids in the map")
	assert.Equal(t, resultMap["uid678"], []string{"uid567"}, "There should be 1 related uid in the map")

	//uid123 should not get newly mapped to any other uids as the currSearchUidsMap and path don't intersect
	resolver.updResultToCurrSearchUidsMap("uid678", map[string]struct{}{"uid567": {}, "uid789": {}},
		resultMap, []string{"uid234", "uid678"})
	assert.Equal(t, len(resultMap), 2, "There should be two uids in the map")
	assert.Equal(t, len(resultMap["uid678"]), 1, "There should be two related uids in the map")
	assert.Equal(t, resultMap["uid678"], []string{"uid567"}, "There should be 1 related uid in the map")
}
