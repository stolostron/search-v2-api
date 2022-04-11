// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stolostron/search-v2-api/graph/model"
)

func Test_SearchSchema_Query(t *testing.T) {
	// Create a SearchSchemaResolver instance with a mock connection pool.
	resolver, _ := newMockSearchSchema(t)

	sql := `SELECT DISTINCT jsonb_object_keys(jsonb_strip_nulls("data")) FROM "search"."resources"`
	// Execute function
	resolver.searchSchemaQuery(context.TODO())

	// Verify response
	if resolver.query != sql {
		t.Errorf("Expected sql guery: %s but got %s", sql, resolver.query)
	}
}

func Test_SearchSchema_Results(t *testing.T) {
	// Create a SearchSchemaResolver instance with a mock connection pool.
	searchInput := &model.SearchInput{}
	resolver, mockPool := newMockSearchSchema(t)

	expectedList := []string{"cluster", "kind", "label", "name", "namespace", "status", "Template", "ReplicaSet"}

	expectedRes := map[string]interface{}{
		"allProperties": expectedList,
	}

	// Mock the database queries.
	mockRows := newMockRows("../resolver/mocks/mock.json", searchInput)
	// Mock the database query
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT jsonb_object_keys(jsonb_strip_nulls("data")) FROM "search"."resources"`),
	).Return(mockRows, nil)
	resolver.searchSchemaQuery(context.TODO())
	res, _ := resolver.searchSchemaResults()
	fmt.Println("results: ", res)
	fmt.Println("expectedRes: ", expectedRes)

	// AssertStringArrayEqual(t, res["allProperties"].([]*string), expectedRes["allProperties"].([]string), "Search schema results doesn't match.")
	if !reflect.DeepEqual(expectedRes, res) {
		t.Errorf("Search schema results doesn't match. Expected: %#v, Got: %#v", expectedRes, res)

	}
}
