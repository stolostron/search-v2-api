// Copyright Contributors to the Open Cluster Management project
package schema

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
	expectedProps := []*string{&val1, &val2}
	// mockRow := &Row{MockValue: expectedProps}
	var expectValues []interface{}
	expectValues = append(expectValues, "kind")
	expectValues = append(expectValues, 10000)
	// Mock the database queries.
	mockRows := newMockRows("../resolver/mocks/mock.json")
	// Mock the database query
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq("SELECT DISTINCT data->> '$1'  FROM search.resources  WHERE  data->>'$1'  IS NOT NULL ORDER BY  data->>'$1' LIMIT $2"),
		gomock.Eq(expectValues)).Return(mockRows, nil)

	// Execute function
	result, _ := resolver.autoComplete(context.TODO())

	// Verify response
	if !string_array_equal(result, expectedProps) {
		t.Errorf("Incorrect Result() expected [%v] got [%v]", expectedProps, result)
	}
}

func string_array_equal(result, expected []*string) bool { //, expected []interface{}) bool {
	for i, exp := range expected {
		if *result[i] != *exp {
			return false
		}
	}
	return true
}
