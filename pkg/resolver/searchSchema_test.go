// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/rbac"
)

func Test_SearchSchema_Query(t *testing.T) {
	// Create a SearchSchemaResolver instance with a mock connection pool.
	resolver, _ := newMockSearchSchema(t)

	resolver.userData = &rbac.UserData{}
	sql := `SELECT DISTINCT "prop" FROM (SELECT jsonb_object_keys(jsonb_strip_nulls("data")) AS "prop" FROM "search"."resources" WHERE (("cluster" = ANY (NULL)) OR ((data->>'_hubClusterResource' = 'true') AND NULL)) LIMIT 100000) AS "schema"`
	// Execute function
	resolver.buildSearchSchemaQuery(context.TODO())

	// Verify response
	if resolver.query != sql {
		t.Errorf("Expected sql query: %s but got %s", sql, resolver.query)
	}
}

func Test_SearchSchema_Results(t *testing.T) {
	// Create a SearchSchemaResolver instance with a mock connection pool.
	searchInput := &model.SearchInput{}
	resolver, mockPool := newMockSearchSchema(t)
	csRes, nsRes, managedClusters := newUserData()
	ud := rbac.UserData{CsResources: csRes, NsResources: nsRes, ManagedClusters: managedClusters}
	resolver.userData = &ud

	expectedList := []string{"cluster", "kind", "label", "name", "namespace", "status", "Template", "ReplicaSet", "ConfigMap"}

	expectedRes := map[string]interface{}{
		"allProperties": expectedList,
	}

	// Mock the database queries.
	//SELECT DISTINCT "prop" FROM (SELECT jsonb_object_keys(jsonb_strip_nulls("data")) AS "prop" FROM "search"."resources" LIMIT 100000) AS "schema"
	mockRows := newMockRowsWithoutRBAC("../resolver/mocks/mock.json", searchInput, "kind", 0)
	// Mock the database query
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "prop" FROM (SELECT jsonb_object_keys(jsonb_strip_nulls("data")) AS "prop" FROM "search"."resources" WHERE (("cluster" = ANY ('{"managed1","managed2"}')) OR ((data->>'_hubClusterResource' = 'true') AND (((COALESCE(data->>'namespace', '') = '') AND (((COALESCE(data->>'apigroup', '') = '') AND (data->>'kind_plural' = 'nodes')) OR ((COALESCE(data->>'apigroup', '') = 'storage.k8s.io') AND (data->>'kind_plural' = 'csinodes')))) OR (((data->>'namespace' = 'default') AND (((COALESCE(data->>'apigroup', '') = '') AND (data->>'kind_plural' = 'configmaps')) OR ((COALESCE(data->>'apigroup', '') = 'v4') AND (data->>'kind_plural' = 'services')))) OR ((data->>'namespace' = 'ocm') AND (((COALESCE(data->>'apigroup', '') = 'v1') AND (data->>'kind_plural' = 'pods')) OR ((COALESCE(data->>'apigroup', '') = 'v2') AND (data->>'kind_plural' = 'deployments')))))))) LIMIT 100000) AS "schema"`),
	).Return(mockRows, nil)
	resolver.buildSearchSchemaQuery(context.TODO())
	res, _ := resolver.searchSchemaResults(context.TODO())

	result := stringArrayToPointer(res["allProperties"].([]string))
	expectedResult := stringArrayToPointer(expectedRes["allProperties"].([]string))

	AssertStringArrayEqual(t, result, expectedResult, "Search schema results doesn't match.")
}
