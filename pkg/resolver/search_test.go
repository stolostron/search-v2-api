// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stolostron/search-v2-api/graph/model"
)

func Test_SearchResolver_Count(t *testing.T) {
	// Create a SearchResolver instance with a mock connection pool.
	val1 := "pod"
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{&model.SearchFilter{Property: "kind", Values: []*string{&val1}}}}
	resolver, mockPool := newMockSearchResolver(t, searchInput)

	// Mock the database query
	mockRow := &Row{MockValue: 10}
	mockPool.EXPECT().QueryRow(gomock.Any(),
		gomock.Eq("SELECT count(uid) FROM search.resources WHERE lower(data->> 'kind')=$1"),
		gomock.Eq("pod")).Return(mockRow)

	// Execute function
	r := resolver.Count()

	// Verify response
	if r != mockRow.MockValue {
		t.Errorf("Incorrect Count() expected [%d] got [%d]", mockRow.MockValue, r)
	}
}

func Test_SearchResolver_Items(t *testing.T) {
	// Create a SearchResolver instance with a mock connection pool.
	val1 := "Template"
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{&model.SearchFilter{Property: "kind", Values: []*string{&val1}}}}
	resolver, mockPool := newMockSearchResolver(t, searchInput)

	// Mock the database queries.
	mockRows := newMockRows("./mocks/mock.json", "Test_SearchResolver_Items")
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq("SELECT uid, cluster, data FROM search.resources WHERE lower(data->> 'kind')=$1"),
		gomock.Eq("template"),
	).Return(mockRows, nil)

	// Execute the function
	result := resolver.Items()

	// Verify returned items.
	if len(result) != len(mockRows.mockData) {
		t.Errorf("Items() received incorrect number of items. Expected %d Got: %d", len(mockRows.mockData), len(result))
	}

	// Verify properties for each returned item.
	for i, item := range result {
		mockRow := mockRows.mockData[i]
		expectedRow := formatDataMap(mockRow["data"].(map[string]interface{}))
		expectedRow["_uid"] = mockRow["uid"]
		expectedRow["cluster"] = mockRow["cluster"]

		if len(item) != len(expectedRow) {
			t.Errorf("Number of properties don't match for item[%d]. Expected: %d Got: %d", i, len(expectedRow), len(item))
		}

		for key, val := range item {
			if val != expectedRow[key] {
				t.Errorf("Value of key [%s] does not match for item [%d].\nExpected: %s\nGot: %s", key, i, expectedRow[key], val)
			}
		}
	}
}

func Test_SearchResolver_Relationships(t *testing.T) {

	val1 := "Pod"
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{&model.SearchFilter{Property: "kind", Values: []*string{&val1}}}}
	resolver, mockPool := newMockSearchResolver(t, searchInput)

	// Mock the database queries.
	mockRows := newMockRows("./mocks/mock.json", "Test_SearchResolver_Relationships")
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq("SELECT uid FROM search.resources WHERE lower(data->> 'kind')=$1"), //we want the output of this query to be the input of the relatinship query
		gomock.Eq("pod"),
	).Return(mockRows, nil)

	//execute the function:

	results := resolver.Uids()

	// verify number of uids == mock uids:
	if len(results) != len(mockRows.mockData) {
		t.Errorf("Items() received incorrect number of items. Expected %d Got: %d", len(mockRows.mockData), len(results))

	}

	//mocking the relationship query results and verifying:
	//val2 := []*string{results}//take the uids from above as input to the searchinput model to get related..
	// searchInput2 := &model.SearchInput{Filters: []*model.SearchFilter{&model.SearchFilter{Values: *results}}}
	// resolver, mockPool := newMockSearchResolver(t, searchInput2)

	mockRows = newMockRows("./mocks/mock-rel.json", "Test_SearchResolver_Relationships")
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`WITH RECURSIVE 
		search_graph(uid, data, sourcekind, destkind, sourceid, destid, path, level)
		AS (
		SELECT r.uid, r.data, e.sourcekind, e.destkind, e.sourceid, e.destid, ARRAY[r.uid] AS path, 1 AS level
			FROM search.resources r
			INNER JOIN
				search.edges e ON (r.uid = e.sourceid) OR (r.uid = e.destid)
			 WHERE r.uid = ANY($1)
		UNION
		SELECT r.uid, r.data, e.sourcekind, e.destkind, e.sourceid, e.destid, path||r.uid, level+1 AS level
			FROM search.resources r
			INNER JOIN
				search.edges e ON (r.uid = e.sourceid)
			, search_graph sg
			WHERE (e.sourceid = sg.destid OR e.destid = sg.sourceid)
			AND r.uid <> all(sg.path)
			AND level = 1 
			)
		SELECT distinct ON (destid) data, destid, destkind FROM search_graph WHERE level=1 OR destid = ANY($2)`),
		gomock.Eq(results),
	).Return(mockRows, nil)

	result2 := resolver.Related()

	// verify number of uids == mock uids:
	if len(result2) != len(mockRows.mockData) {
		t.Errorf("Items() received incorrect number of items. Expected %d Got: %d", len(mockRows.mockData), len(result2))

	}

	// Verify properties for each returned item.
	// destid
	// data <---don't need?..
	// destkind

}
