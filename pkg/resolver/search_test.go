// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
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

func Test_SearchResolver_CountWithOperator(t *testing.T) {
	// Create a SearchResolver instance with a mock connection pool.
	val1 := ">=1"
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "current", Values: []*string{&val1}}}}
	resolver, mockPool := newMockSearchResolver(t, searchInput, nil)

	// Mock the database query
	mockRow := &Row{MockValue: 1}
	mockPool.EXPECT().QueryRow(gomock.Any(),
		gomock.Eq(`SELECT COUNT("uid") FROM "search"."resources" WHERE ("data"->>'current' >= ('1'))`),
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
	mockRows := newMockRows("./mocks/mock.json", searchInput, "")

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

type TestOperatorItem struct {
	searchInput *model.SearchInput
	mockQuery   string
}

func Test_SearchResolver_ItemsWithOperator(t *testing.T) {

	//define schema table:
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)

	val1 := ">1"
	testOperatorGreater := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: "current", Values: []*string{&val1}}}},
		mockQuery:   `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->>'current' > ('1')) LIMIT 10000`,
	}
	val2 := "<4"
	testOperatorLesser := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: "current", Values: []*string{&val2}}}},
		mockQuery:   `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->>'current' < ('4')) LIMIT 10000`,
	}
	val3 := ">=1"
	testOperatorGreaterorEqual := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: "current", Values: []*string{&val3}}}},
		mockQuery:   `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->>'current' >= ('1')) LIMIT 10000`,
	}
	val4 := "<=3"
	testOperatorLesserorEqual := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: "current", Values: []*string{&val4}}}},
		mockQuery:   `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->>'current' <= ('3')) LIMIT 10000`,
	}

	val5 := "!4"
	testOperatorNot := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: "current", Values: []*string{&val5}}}},
		mockQuery:   `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->>'current' NOT IN ('4')) LIMIT 10000`,
	}

	val6 := "!=4"
	testOperatorNotEqual := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: "current", Values: []*string{&val6}}}},
		mockQuery:   `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->>'current' NOT IN ('4')) LIMIT 10000`,
	}

	val7 := "=3"
	testOperatorEqual := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: "current", Values: []*string{&val7}}}},
		mockQuery:   `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->>'current' IN ('3')) LIMIT 10000`,
	}

	val8 := "year"
	_, newVal8 := getDateFilter([]string{val8})
	mockQueryYear, _, _ := ds.Select("uid", "cluster", "data").Where(goqu.L(`"data"->>?`, "created").Gt(goqu.L("?", newVal8))).Limit(10000).ToSQL()

	testOperatorYear := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: "created", Values: []*string{&val8}}}},
		mockQuery:   mockQueryYear, // `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->>'created' > ('2021-05-16T13:11:12Z')) LIMIT 10000`,
	}

	val9 := "hour"
	_, newVal9 := getDateFilter([]string{val9})
	mockQueryHour, _, _ := ds.Select("uid", "cluster", "data").Where(goqu.L(`"data"->>?`, "created").Gt(goqu.L("?", newVal9))).Limit(10000).ToSQL()

	testOperatorHour := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: "created", Values: []*string{&val9}}}},
		mockQuery:   mockQueryHour, // `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->>'created' > ('2021-05-16T13:11:12Z')) LIMIT 10000`,
	}

	val10 := "day"
	_, newVal10 := getDateFilter([]string{val10})
	mockQueryDay, _, _ := ds.Select("uid", "cluster", "data").Where(goqu.L(`"data"->>?`, "created").Gt(goqu.L("?", newVal10))).Limit(10000).ToSQL()

	testOperatorDay := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: "created", Values: []*string{&val10}}}},
		mockQuery:   mockQueryDay, // `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->>'created' > ('2021-05-16T13:11:12Z')) LIMIT 10000`,
	}

	val11 := "week"
	_, newVal11 := getDateFilter([]string{val11})
	mockQueryWeek, _, _ := ds.Select("uid", "cluster", "data").Where(goqu.L(`"data"->>?`, "created").Gt(goqu.L("?", newVal11))).Limit(10000).ToSQL()

	testOperatorWeek := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: "created", Values: []*string{&val11}}}},
		mockQuery:   mockQueryWeek, // `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->>'created' > ('2021-05-16T13:11:12Z')) LIMIT 10000`,
	}

	val12 := "month"
	_, newVal12 := getDateFilter([]string{val12})
	mockQueryMonth, _, _ := ds.Select("uid", "cluster", "data").Where(goqu.L(`"data"->>?`, "created").Gt(goqu.L("?", newVal12))).Limit(10000).ToSQL()

	testOperatorMonth := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: "created", Values: []*string{&val12}}}},
		mockQuery:   mockQueryMonth, // `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->>'created' > ('2021-05-16T13:11:12Z')) LIMIT 10000`,
	}

	testOperators := []TestOperatorItem{
		testOperatorGreater, testOperatorLesser, testOperatorGreaterorEqual,
		testOperatorLesserorEqual, testOperatorNot, testOperatorNotEqual, testOperatorEqual,
		testOperatorYear, testOperatorHour, testOperatorDay, testOperatorWeek, testOperatorMonth,
	}

	for _, currTest := range testOperators {
		// Create a SearchResolver instance with a mock connection pool.
		resolver, mockPool := newMockSearchResolver(t, currTest.searchInput, nil)

		// Mock the database queries.
		mockRows := newMockRows("./mocks/mock.json", currTest.searchInput, "")

		mockPool.EXPECT().Query(gomock.Any(),
			gomock.Eq(currTest.mockQuery),
			gomock.Eq([]interface{}{}),
		).Return(mockRows, nil)

		// Execute the function
		result := resolver.Items()
		// Verify returned items.
		if len(result) != len(mockRows.mockData) {
			t.Errorf("Items() received incorrect number of items. Expected %d Got: %d", len(mockRows.mockData), len(result))
		}

		// // Verify properties for each returned item.
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
	mockRows := newMockRows("./mocks/mock.json", searchInput, "")
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
	mockRows := newMockRows("../resolver/mocks/mock.json", searchInput, "")

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

	relQuery := strings.TrimSpace(`WITH RECURSIVE search_graph(uid, data, destkind, sourceid, destid, path, level) AS (SELECT "r"."uid", "r"."data", "e"."destkind", "e"."sourceid", "e"."destid", ARRAY[r.uid] AS "path", 1 AS "level" FROM "search"."resources" AS "r" INNER JOIN "search"."edges" AS "e" ON ("r"."uid" IN ("e"."sourceid", "e"."destid")) WHERE ("r"."uid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b')) UNION (SELECT "r"."uid", "r"."data", "e"."destkind", "e"."sourceid", "e"."destid", sg.path||r.uid AS "path", level+1 AS "level" FROM "search"."resources" AS "r" INNER JOIN "search"."edges" AS "e" ON ("r"."uid" = "e"."sourceid") INNER JOIN "search_graph" AS "sg" ON (("sg"."destid" = "e"."sourceid") OR ("sg"."sourceid" = "e"."destid")) WHERE (("r"."uid" != ALL ('{sg.path}')) AND ("sg"."level" = 1)))) SELECT DISTINCT ON ("destid") "data", "destid", "destkind" FROM "search_graph" WHERE ("level" = 1)`)

	mockRows := newMockRows("./mocks/mock-rel-1.json", searchInput, "")
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
		destKind, _ := data["destkind"].(string)
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

	relQuery := strings.TrimSpace(`WITH RECURSIVE search_graph(uid, data, destkind, sourceid, destid, path, level) AS (SELECT "r"."uid", "r"."data", "e"."destkind", "e"."sourceid", "e"."destid", ARRAY[r.uid] AS "path", 1 AS "level" FROM "search"."resources" AS "r" INNER JOIN "search"."edges" AS "e" ON ("r"."uid" IN ("e"."sourceid", "e"."destid")) WHERE ("r"."uid" IN ('local-cluster/e12c2ddd-4ac5-499d-b0e0-20242f508afd', 'local-cluster/13250bc4-865c-41db-a8f2-05bec0bd042b')) UNION (SELECT "r"."uid", "r"."data", "e"."destkind", "e"."sourceid", "e"."destid", sg.path||r.uid AS "path", level+1 AS "level" FROM "search"."resources" AS "r" INNER JOIN "search"."edges" AS "e" ON ("r"."uid" = "e"."sourceid") INNER JOIN "search_graph" AS "sg" ON (("sg"."destid" = "e"."sourceid") OR ("sg"."sourceid" = "e"."destid")) WHERE (("r"."uid" != ALL ('{sg.path}')) AND ("sg"."level" = 1)))) SELECT DISTINCT ON ("destid") "data", "destid", "destkind" FROM "search_graph" WHERE (("destkind" IN ('ConfigMap')) AND ("level" = 1))`)

	mockRows := newMockRows("./mocks/mock-rel-1.json", searchInput2, "")
	mockPool2.EXPECT().Query(gomock.Any(),
		gomock.Eq(relQuery),
		gomock.Eq([]interface{}{}),
	).Return(mockRows, nil)

	result := resolver.Related() // this should return a relatedResults object

	if result[0].Kind != mockRows.mockData[0]["destkind"] {
		t.Errorf("Kind value in mockdata does not match kind value of result")
	}

	// Verify returned items.
	if len(result) != len(mockRows.mockData) {
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
	mockRows := newMockRows("./mocks/mock.json", searchInput, "")

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
