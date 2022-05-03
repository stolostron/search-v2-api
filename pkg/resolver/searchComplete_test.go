// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stolostron/search-v2-api/graph/model"
)

func Test_SearchComplete_Query(t *testing.T) {
	// Create a SearchCompleteResolver instance with a mock connection pool.
	prop1 := "kind"
	searchInput := &model.SearchInput{}
	resolver, mockPool := newMockSearchComplete(t, searchInput, prop1)
	val1 := "Template"
	val2 := "ReplicaSet"
	val3 := "ConfigMap"
	expectedProps := []*string{&val1, &val2, &val3}

	// Mock the database queries.
	mockRows := newMockRows("../resolver/mocks/mock.json", searchInput)
	// Mock the database query
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "data"->>'kind' FROM "search"."resources" WHERE ("data"->>'kind' IS NOT NULL) ORDER BY "data"->>'kind' ASC LIMIT 10000`),
		gomock.Eq([]interface{}{})).Return(mockRows, nil)

	// Execute function
	result, err := resolver.autoComplete(context.TODO())
	if err != nil {
		t.Errorf("Incorrect results. expected error to be [%v] got [%v]", nil, err)

	}
	// Verify response
	AssertStringArrayEqual(t, result, expectedProps, "Error in Test_SearchComplete_Query")
}

func Test_SearchCompleteNoProp_Query(t *testing.T) {
	// Create a SearchCompleteResolver instance with a mock connection pool.
	prop1 := ""
	searchInput := &model.SearchInput{}
	resolver, mockPool := newMockSearchComplete(t, searchInput, prop1)
	val1 := "Template"
	val2 := "ReplicaSet"
	val3 := "ConfigMap"
	expectedProps := []*string{&val1, &val2, &val3}

	// Mock the database queries.
	mockRows := newMockRows("../resolver/mocks/mock.json", searchInput)
	// Mock the database query
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(""),
		gomock.Eq([]interface{}{})).Return(mockRows, nil)

	// Execute function
	result, _ := resolver.autoComplete(context.TODO())

	// Verify response
	AssertStringArrayEqual(t, result, expectedProps, "Error in Test_SearchCompleteNoProp_Query")
}

func Test_SearchCompleteWithFilter_Query(t *testing.T) {
	// Create a SearchCompleteResolver instance with a mock connection pool.
	prop1 := "cluster"
	value1 := "openshift"
	value2 := "openshift-monitoring"
	cluster := "local-cluster"
	limit := 10
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "namespace", Values: []*string{&value1, &value2}}, {Property: "cluster", Values: []*string{&cluster}}}, Limit: &limit}
	resolver, mockPool := newMockSearchComplete(t, searchInput, prop1)
	val1 := "Template"
	val2 := "ReplicaSet"
	val3 := "ConfigMap"
	expectedProps := []*string{&val1, &val2, &val3}

	// Mock the database queries.
	mockRows := newMockRows("../resolver/mocks/mock.json", searchInput)
	// Mock the database query
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "cluster" FROM "search"."resources" WHERE (("data"->>'namespace' IN ('openshift', 'openshift-monitoring')) AND ("cluster" IN ('local-cluster')) AND ("cluster" IS NOT NULL)) ORDER BY "cluster" ASC LIMIT 10`),
		gomock.Eq([]interface{}{})).Return(mockRows, nil)

	// Execute function
	result, _ := resolver.autoComplete(context.TODO())

	// Verify response
	AssertStringArrayEqual(t, result, expectedProps, "Error in Test_SearchCompleteWithFilter_Query")
}
