// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stolostron/search-v2-api/graph/model"
)

func Test_SearchResolver_Count(t *testing.T) {
	// Create a SearchResolver instance with a mock connection pool.
	val1 := "pod"
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{&model.SearchFilter{Property: "kind", Values: []*string{&val1}}}}
	resolver, mockPool := newMockSearchResolver(t, searchInput, nil)

	// Mock the database query
	mockRow := &Row{MockValue: 10}
	mockPool.EXPECT().QueryRow(gomock.Any(),
		gomock.Eq("SELECT count(uid) FROM search.resources  WHERE lower(data->> 'kind')=$1"),
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
	val1 := "template"
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{&model.SearchFilter{Property: "kind", Values: []*string{&val1}}}}
	resolver, mockPool := newMockSearchResolver(t, searchInput, nil)
	// Mock the database queries.
	mockRows := newMockRows("./mocks/mock.json")

	t.Log("MOCK ROWS are:", mockRows.mockData)
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq("SELECT uid, cluster, data FROM search.resources  WHERE lower(data->> 'kind')=$1"),
		gomock.Eq("template"),
	).Return(mockRows, nil)

	// Execute the function
	result := resolver.Items()

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

	var resultList []*string

	uid1 := "local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd"
	uid2 := "local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b"

	resultList = append(resultList, &uid1, &uid2)

	// //take the uids from above as input
	searchInput2 := &model.SearchInput{Filters: []*model.SearchFilter{&model.SearchFilter{Property: "uid", Values: resultList}}}
	resolver2, mockPool2 := newMockSearchResolver(t, searchInput2, resultList)

	relQuery := strings.TrimSpace(`WITH RECURSIVE
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
	SELECT distinct ON (destid) data, destid, destkind FROM search_graph WHERE level=1 OR destid = ANY($1)`)

	mockRows := newMockRows("./mocks/mock-rel-1.json")
	mockPool2.EXPECT().Query(gomock.Any(),
		gomock.Eq(relQuery),
		gomock.Eq(resultList),
	).Return(mockRows, nil)

	result2 := resolver2.Related() // this should return a relatedResults object

	if result2[0].Kind != mockRows.mockData[0]["destkind"] {
		t.Errorf("Kind value in mockdata does not match kind value of result")
	}

}
