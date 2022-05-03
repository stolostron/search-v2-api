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
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}}}
	resolver, mockPool := newMockSearchResolver(t, searchInput, nil)

	// Mock the database query
	mockRow := &Row{MockValue: 10}
	mockPool.EXPECT().QueryRow(gomock.Any(),
		gomock.Eq(`SELECT COUNT("uid") FROM "search"."resources" WHERE ("data"->>'kind' IN ('pod'))`),
		gomock.Eq([]interface{}{})).Return(mockRow)

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
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}}}
	resolver, mockPool := newMockSearchResolver(t, searchInput, nil)
	// Mock the database queries.
	mockRows := newMockRows("./mocks/mock.json", searchInput)

	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->>'kind' IN ('template')) LIMIT 10000`),
		gomock.Eq([]interface{}{}),
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

func Test_SearchResolver_Items_Multiple_Filter(t *testing.T) {
	// Create a SearchResolver instance with a mock connection pool.
	val1 := "openshift"
	val2 := "openshift-monitoring"
	cluster := "local-cluster"
	limit := 10
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "namespace", Values: []*string{&val1, &val2}}, {Property: "cluster", Values: []*string{&cluster}}}, Limit: &limit}
	resolver, mockPool := newMockSearchResolver(t, searchInput, nil)

	// Mock the database queries.
	mockRows := newMockRows("./mocks/mock.json", searchInput)
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE (("data"->>'namespace' IN ('openshift', 'openshift-monitoring')) AND ("cluster" IN ('local-cluster'))) LIMIT 10`),
		// gomock.Eq("SELECT uid, cluster, data FROM search.resources  WHERE lower(data->> 'namespace')=any($1) AND cluster=$2 LIMIT 10"),
		gomock.Eq([]interface{}{}),
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

func Test_SearchWithMultipleClusterFilter_NegativeLimit_Query(t *testing.T) {
	// Create a SearchResolver instance with a mock connection pool.
	value1 := "openshift"
	cluster1 := "local-cluster"
	cluster2 := "remote-1"
	limit := -1
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "namespace", Values: []*string{&value1}}, {Property: "cluster", Values: []*string{&cluster1, &cluster2}}}, Limit: &limit}
	resolver, mockPool := newMockSearchResolver(t, searchInput, nil)

	// Mock the database queries.
	mockRows := newMockRows("../resolver/mocks/mock.json", searchInput)

	// Mock the database query
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE (("data"->>'namespace' IN ('openshift')) AND ("cluster" IN ('local-cluster', 'remote-1')))`),
		gomock.Eq([]interface{}{})).Return(mockRows, nil)

	// Execute function
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

	var resultList []*string

	uid1 := "local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd"
	uid2 := "local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b"

	resultList = append(resultList, &uid1, &uid2)

	// //take the uids from above as input
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "uid", Values: resultList}}}
	resolver, mockPool2 := newMockSearchResolver(t, searchInput, resultList)

	relQuery := strings.TrimSpace(`SELECT "iid", "kind", MIN("level") AS "level" FROM (SELECT "level", unnest(array[sourceid, destid, concat('cluster__',cluster)]) AS "iid", unnest(array[sourcekind, destkind, 'Cluster']) AS "kind" FROM (WITH RECURSIVE search_graph(level, sourceid, destid,  sourcekind, destkind, cluster) AS (SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster" FROM "search"."all_edges" AS "e" WHERE (("destid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b')) OR ("sourceid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b'))) UNION (SELECT level+1 AS "level", "e"."sourceid", "e"."destid", "e"."sourcekind", "e"."destkind", "e"."cluster" FROM "search"."all_edges" AS "e" INNER JOIN "search_graph" AS "sg" ON (("sg"."destid" IN ("e"."sourceid", "e"."destid")) OR ("sg"."sourceid" IN ("e"."sourceid", "e"."destid"))) WHERE (("e"."destkind" != 'Node') AND ("sg"."level" < 4)))) SELECT DISTINCT "level", "sourceid", "destid", "sourcekind", "destkind", "cluster" FROM "search_graph") AS "search_graph") AS "combineIds" WHERE (("level" < 4) AND ("iid" NOT IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b'))) GROUP BY "iid", "kind"`)

	mockRows := newMockRows("./mocks/mock-rel-1.json", searchInput)
	mockPool2.EXPECT().Query(gomock.Any(),
		gomock.Eq(relQuery),
		gomock.Eq([]interface{}{}),
	).Return(mockRows, nil)

	result := resolver.Related() // this should return a relatedResults object

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

	var resultList []*string

	uid1 := "local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd"
	uid2 := "local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b"

	resultList = append(resultList, &uid1, &uid2)
	relatedKind1 := "ConfigMap"
	// //take the uids from above as input
	searchInput2 := &model.SearchInput{RelatedKinds: []*string{&relatedKind1}, Filters: []*model.SearchFilter{{Property: "uid", Values: resultList}}}
	resolver, mockPool2 := newMockSearchResolver(t, searchInput2, resultList)

	relQuery := strings.TrimSpace(`SELECT "iid", "kind", MIN("level") AS "level" FROM (SELECT "level", unnest(array[sourceid, destid, concat('cluster__',cluster)]) AS "iid", unnest(array[sourcekind, destkind, 'Cluster']) AS "kind" FROM (WITH RECURSIVE search_graph(level, sourceid, destid,  sourcekind, destkind, cluster) AS (SELECT 1 AS "level", "sourceid", "destid", "sourcekind", "destkind", "cluster" FROM "search"."all_edges" AS "e" WHERE (("destid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b')) OR ("sourceid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b'))) UNION (SELECT level+1 AS "level", "e"."sourceid", "e"."destid", "e"."sourcekind", "e"."destkind", "e"."cluster" FROM "search"."all_edges" AS "e" INNER JOIN "search_graph" AS "sg" ON (("sg"."destid" IN ("e"."sourceid", "e"."destid")) OR ("sg"."sourceid" IN ("e"."sourceid", "e"."destid"))) WHERE (("e"."destkind" != 'Node') AND ("sg"."level" < 4)))) SELECT DISTINCT "level", "sourceid", "destid", "sourcekind", "destkind", "cluster" FROM "search_graph") AS "search_graph") AS "combineIds" WHERE (("level" < 4) AND ("iid" NOT IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b'))) GROUP BY "iid", "kind"`)

	mockRows := newMockRows("./mocks/mock-rel-1.json", searchInput2)
	mockPool2.EXPECT().Query(gomock.Any(),
		gomock.Eq(relQuery),
		gomock.Eq([]interface{}{}),
	).Return(mockRows, nil)
	mockRows2 := newMockRows("./mocks/mock.json", searchInput2)

	relatedQuery := `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("uid" IN ('local-cluster/30c35f12-320a-417f-98d1-fbee28a4b2a6')) LIMIT 10000`
	// Mock the database query
	mockPool2.EXPECT().Query(gomock.Any(),
		gomock.Eq(relatedQuery),
		gomock.Eq([]interface{}{}),
	).Return(mockRows2, nil)

	result := resolver.Related() // this should return a relatedResults object

	if !strings.EqualFold(result[0].Kind, strings.ToLower(mockRows2.mockData[0]["destkind"].(string))) {
		t.Errorf("Kind value in mockdata does not match kind value of result")
	}

	// Verify returned items.
	if len(result) != len(mockRows2.mockData) {
		t.Errorf("Items() received incorrect number of items. Expected %d Got: %d", len(mockRows.mockData), len(result))
	}
}

func Test_SearchResolver_Keywords(t *testing.T) {
	// Create a SearchResolver instance with a mock connection pool.
	val1 := "Template"
	limit := 10
	searchInput := &model.SearchInput{Keywords: []*string{&val1}, Limit: &limit}
	resolver, mockPool := newMockSearchResolver(t, searchInput, nil)

	// Mock the database queries.
	mockRows := newMockRows("./mocks/mock.json", searchInput)

	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT "uid", "cluster", "data" FROM "search"."resources", jsonb_each_text("data") WHERE ("value" LIKE '%Template%') LIMIT 10`),
		gomock.Eq([]interface{}{}),
	).Return(mockRows, nil)

	// // Execute the function
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
