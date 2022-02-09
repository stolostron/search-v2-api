// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"fmt"
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

	t.Log("Print")

	// Mock the database queries.
	mockRows := newMockRows("non-rel")
	t.Logf("MOCK ROWS TYPE IS: %T", mockRows)
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

	//mock input, build mockResovler and mockPool
	// val1 := "Pod"
	// searchInput := &model.SearchInput{Filters: []*model.SearchFilter{&model.SearchFilter{Property: "kind", Values: []*string{&val1}}}}
	// resolver, mockPool := newMockSearchResolver(t, searchInput)
	// fmt.Println("After creating a resolver and mockpool for mock search input.")

	// // Mock the database queries.
	// mockRows := newMockRows("non-rel")
	// mockPool.EXPECT().Query(gomock.Any(),
	// 	gomock.Eq("SELECT uid FROM search.resources WHERE lower(data->> 'kind')=$1"), //we want the output of this query to be the input of the relatinship query
	// 	gomock.Eq("pod"),
	// ).Return(mockRows, nil)
	// fmt.Println("After mocking data queries for initial query.")

	// //execute the function/ this will need to be passed to the recursive query:
	// results := resolver.Uids()
	// fmt.Println(results)
	// fmt.Println("After results")
	// // verify number of uids == mock uids:
	// if len(results) != len(mockRows.mockData) {
	// 	t.Errorf("Items() received incorrect number of items. Expected %d Got: %d", len(mockRows.mockData), len(results))

	// }
	// fmt.Println("After verifying.")

	resultString := "[local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd, local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b]"

	var resultList []string

	resultList = append(resultList, "local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd", "local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b")

	// //take the uids from above as input
	searchInput2 := &model.SearchInput{Filters: []*model.SearchFilter{&model.SearchFilter{Property: "uid", Values: []*string{&resultString}}}}
	resolver2, mockPool2 := newMockSearchResolver(t, searchInput2)

	mockRows := newMockRows("rel")
	fmt.Println("Mock Rows are:", mockRows)
	mockPool2.EXPECT().Query(gomock.Any(),
		gomock.Eq(`WITH RECURSIVE
		search_graph(uid, data, destkind, sourceid, destid, path, level)
		AS (
		SELECT r.uid, r.data, e.destkind, e.sourceid, e.destid, ARRAY[r.uid] AS path, 1 AS level
			FROM search.resources r
			INNER JOIN
				search.edges e ON (r.uid = e.sourceid) OR (r.uid = e.destid)
			 WHERE r.uid = ANY($1)
		UNION
		SELECT r.uid, r.data, e.destkind, e.sourceid, e.destid, path||r.uid, level+1 AS level
			FROM search.resources r
			INNER JOIN
				search.edges e ON (r.uid = e.sourceid)
			, search_graph sg
			WHERE (e.sourceid = sg.destid OR e.destid = sg.sourceid)
			AND r.uid <> all(sg.path)
			AND level = 1
			)
		SELECT distinct ON (destid) data, destid, destkind FROM search_graph WHERE level=1 OR destid = ANY($1)`),
		gomock.Eq(resultList),
	).Return(mockRows, nil)

	result2 := resolver2.Related() //here it fails

	fmt.Printf("%T", result2)

	// fmt.Println(result2)
	// // verify number of uids == mock uids:
	// if len(result2) != len(mockRows.mockData) {
	// 	t.Errorf("Items() received incorrect number of items. Expected %d Got: %d", len(mockRows.mockData), len(result2))
	// }

	// Verify properties for each returned item.
	//  for i, item := range result2 {
	// 	mockRow := mockRows.mockData[i]
	// 	expectedRow := formatDataMap(mockRow["data"].(map[string]interface{}))
	// 	expectedRow["destkind"] = mockRow["destkind"]
	// 	expectedRow["destid"] = mockRow["destid"]

	// for key, val := range item {
	// 	if val != expectedRow[key] {
	// 		t.Errorf("Value of key [%s] does not match for item [%d].\nExpected: %s\nGot: %s", key, i, expectedRow[key], val)
	// 	}
	// }
}
