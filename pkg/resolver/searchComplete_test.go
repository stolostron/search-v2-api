// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	"github.com/stretchr/testify/assert"
)

func Test_SearchComplete_Query(t *testing.T) {
	// Create a SearchCompleteResolver instance with a mock connection pool.
	prop1 := "kind"
	searchInput := &model.SearchInput{}
	resolver, mockPool := newMockSearchComplete(t, searchInput, prop1, &rbac.UserData{})
	val1 := "Template"
	val2 := "ReplicaSet"
	val3 := "ConfigMap"
	expectedProps := []*string{&val1, &val2, &val3}

	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("../resolver/mocks/mock.json", searchInput, prop1, 0)
	// Mock the database query
	// SELECT DISTINCT "prop" FROM (SELECT "data"->>'kind' AS "prop" FROM "search"."resources" WHERE ("data"->>'kind' IS NOT NULL) LIMIT 100000) AS "searchComplete" ORDER BY prop ASC LIMIT 1000
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "prop" FROM (SELECT "data"->>'kind' AS "prop" FROM "search"."resources" WHERE (("data"->>'kind' IS NOT NULL) AND (("cluster" = ANY ('{}')) OR ((data->>'_hubClusterResource' = 'true') AND NULL))) LIMIT 100000) AS "searchComplete" ORDER BY prop ASC LIMIT 1000`),
		gomock.Eq([]interface{}{})).Return(mockRows, nil)

	// Execute function
	result, err := resolver.autoComplete(context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456"))
	if err != nil {
		t.Errorf("Incorrect results. expected error to be [%v] got [%v]", nil, err)

	}
	// Verify response
	AssertStringArrayEqual(t, result, expectedProps, "Error in Test_SearchComplete_Query")
}

func Test_SearchComplete_Query_WithLimit(t *testing.T) {
	// Create a SearchCompleteResolver instance with a mock connection pool.
	prop1 := "kind"
	limit := 2
	searchInput := &model.SearchInput{}
	resolver, mockPool := newMockSearchComplete(t, searchInput, prop1, &rbac.UserData{})
	resolver.limit = &limit //Add limit
	val1 := "ConfigMap"
	val2 := "ReplicaSet"
	expectedProps := []*string{&val1, &val2}

	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("../resolver/mocks/mock.json", searchInput, prop1, limit)
	fmt.Println("mockRows:", mockRows)
	// Mock the database query
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "prop" FROM (SELECT "data"->>'kind' AS "prop" FROM "search"."resources" WHERE (("data"->>'kind' IS NOT NULL) AND (("cluster" = ANY ('{}')) OR ((data->>'_hubClusterResource' = 'true') AND NULL))) LIMIT 100000) AS "searchComplete" ORDER BY prop ASC LIMIT 2`),
		gomock.Eq([]interface{}{})).Return(mockRows, nil)

	// Execute function
	result, err := resolver.autoComplete(context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456"))
	if err != nil {
		t.Errorf("Incorrect results. expected error to be [%v] got [%v]", nil, err)

	}
	// Verify response
	AssertStringArrayEqual(t, result, expectedProps, "Error in Test_SearchComplete_Query_WithLimit")
}

func Test_SearchCompleteNoProp_Query(t *testing.T) {
	// Create a SearchCompleteResolver instance with a mock connection pool.
	prop1 := ""
	searchInput := &model.SearchInput{}
	resolver, mockPool := newMockSearchComplete(t, searchInput, prop1, &rbac.UserData{})
	expectedProps := []*string{}

	// Mock the database query
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(""),
		gomock.Eq([]interface{}{})).Return(nil, fmt.Errorf("Error in search complete query. No property specified."))

	// Execute function
	result, err := resolver.autoComplete(context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456"))
	// Verify response
	AssertStringArrayEqual(t, result, expectedProps, "Error in Test_SearchCompleteNoProp_Query")
	assert.NotNil(t, err, "Expected error")
}

func Test_SearchCompleteWithFilter_Query(t *testing.T) {
	// Create a SearchCompleteResolver instance with a mock connection pool.
	prop1 := "kind"
	value1 := "openshift"
	value2 := "openshift-monitoring"
	cluster := "local-cluster"
	limit := 10
	csRes, nsRes, managedClusters := newUserData()
	ud := rbac.UserData{CsResources: csRes, NsResources: nsRes, ManagedClusters: managedClusters}
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "namespace", Values: []*string{&value1, &value2}}, {Property: "cluster", Values: []*string{&cluster}}}, Limit: &limit}
	resolver, mockPool := newMockSearchComplete(t, searchInput, prop1, &ud)
	val1 := "Template"
	val2 := "ReplicaSet"
	val3 := "ConfigMap"
	resolver.limit = &limit
	expectedProps := []*string{&val1, &val2, &val3}

	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("../resolver/mocks/mock.json", searchInput, prop1, limit)
	// Mock the database query
	// check if cluster
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "prop" FROM (SELECT "data"->>'kind' AS "prop" FROM "search"."resources" WHERE (("data"->>'namespace' IN ('openshift', 'openshift-monitoring')) AND ("cluster" IN ('local-cluster')) AND ("data"->>'kind' IS NOT NULL) AND (("cluster" = ANY ('{"managed1","managed2"}')) OR ((data->>'_hubClusterResource' = 'true') AND (((COALESCE(data->>'namespace', '') = '') AND (((COALESCE(data->>'apigroup', '') = '') AND (data->>'kind_plural' = 'nodes')) OR ((COALESCE(data->>'apigroup', '') = 'storage.k8s.io') AND (data->>'kind_plural' = 'csinodes')))) OR (((data->>'namespace' = 'default') AND (((COALESCE(data->>'apigroup', '') = '') AND (data->>'kind_plural' = 'configmaps')) OR ((COALESCE(data->>'apigroup', '') = 'v4') AND (data->>'kind_plural' = 'services')))) OR ((data->>'namespace' = 'ocm') AND (((COALESCE(data->>'apigroup', '') = 'v1') AND (data->>'kind_plural' = 'pods')) OR ((COALESCE(data->>'apigroup', '') = 'v2') AND (data->>'kind_plural' = 'deployments'))))))))) LIMIT 100000) AS "searchComplete" ORDER BY prop ASC LIMIT 10`),
		gomock.Eq([]interface{}{})).Return(mockRows, nil)

	// Execute function
	result, _ := resolver.autoComplete(context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456"))

	// Verify response
	AssertStringArrayEqual(t, result, expectedProps, "Error in Test_SearchCompleteWithFilter_Query")
}

func Test_SearchCompleteWithCluster(t *testing.T) {
	// Create a SearchCompleteResolver instance with a mock connection pool.
	prop1 := "cluster"

	cluster := "local-cluster"
	limit := 10
	searchInput := &model.SearchInput{}

	resolver, mockPool := newMockSearchComplete(t, searchInput, prop1, &rbac.UserData{})
	resolver.limit = &limit
	expectedProps := []*string{&cluster}

	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("../resolver/mocks/mock.json", searchInput, prop1, limit)
	// Mock the database query
	// SELECT DISTINCT "prop" FROM (SELECT DISTINCT "cluster" AS "prop" FROM "search"."resources" WHERE (("cluster" IS NOT NULL) AND ("cluster" != '')) LIMIT 100000) AS "searchComplete" ORDER BY prop ASC LIMIT 10
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "prop" FROM (SELECT DISTINCT "cluster" AS "prop" FROM "search"."resources" WHERE (("cluster" IS NOT NULL) AND ("cluster" != '') AND (("cluster" = ANY ('{}')) OR ((data->>'_hubClusterResource' = 'true') AND NULL))) LIMIT 100000) AS "searchComplete" ORDER BY prop ASC LIMIT 10`),
		gomock.Eq([]interface{}{})).Return(mockRows, nil)

	// Execute function
	result, _ := resolver.autoComplete(context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456"))

	// Verify response
	AssertStringArrayEqual(t, result, expectedProps, "Error in Test_SearchCompleteWithFilter_Query")
}

func Test_SearchCompleteQuery_PropDate(t *testing.T) {
	// Create a SearchCompleteResolver instance with a mock connection pool.
	prop1 := "created"
	searchInput := &model.SearchInput{}
	resolver, mockPool := newMockSearchComplete(t, searchInput, prop1, &rbac.UserData{})
	val1 := "isDate"
	expectedProps := []*string{&val1} //, &val2, &val3}

	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("../resolver/mocks/mock.json", searchInput, prop1, 0)
	fmt.Println("mockRows:", mockRows)
	// Mock the database query
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "prop" FROM (SELECT "data"->>'created' AS "prop" FROM "search"."resources" WHERE (("data"->>'created' IS NOT NULL) AND (("cluster" = ANY ('{}')) OR ((data->>'_hubClusterResource' = 'true') AND NULL))) LIMIT 100000) AS "searchComplete" ORDER BY prop ASC LIMIT 1000`),
		gomock.Eq([]interface{}{})).Return(mockRows, nil)

	// Execute function
	result, err := resolver.autoComplete(context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456"))
	if err != nil {
		t.Errorf("Incorrect results. expected error to be [%v] got [%v]", nil, err)

	}
	// Verify response
	AssertStringArrayEqual(t, result, expectedProps, "Error in Test_SearchCompleteQuery_PropDate")
}

func Test_SearchCompleteQuery_PropNum(t *testing.T) {
	// Create a SearchCompleteResolver instance with a mock connection pool.
	prop1 := "current"
	searchInput := &model.SearchInput{}
	_, _, mc := newUserData()
	resolver, mockPool := newMockSearchComplete(t, searchInput, prop1, &rbac.UserData{ManagedClusters: mc})
	val1 := "isNumber"
	val2 := "3"
	expectedProps := []*string{&val1, &val2} //, &val3}

	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("../resolver/mocks/mock.json", searchInput, prop1, 0)
	// Mock the database query
	// SELECT DISTINCT "prop" FROM (SELECT "data"->>'current' AS "prop" FROM "search"."resources" WHERE ("data"->>'current' IS NOT NULL) LIMIT 100000) AS "searchComplete" ORDER BY prop ASC LIMIT 1000

	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "prop" FROM (SELECT "data"->>'current' AS "prop" FROM "search"."resources" WHERE (("data"->>'current' IS NOT NULL) AND (("cluster" = ANY ('{"managed1","managed2"}')) OR ((data->>'_hubClusterResource' = 'true') AND NULL))) LIMIT 100000) AS "searchComplete" ORDER BY prop ASC LIMIT 1000`),
		gomock.Eq([]interface{}{})).Return(mockRows, nil)

	// Execute function
	result, err := resolver.autoComplete(context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456"))
	if err != nil {
		t.Errorf("Incorrect results. expected error to be [%v] got [%v]", nil, err)

	}
	// Verify response
	AssertStringArrayEqual(t, result, expectedProps, "Error in Test_SearchCompleteQuery_PropNum")
}
