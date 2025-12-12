// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/golang/mock/gomock"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/authentication/v1"
)

func Test_SearchResolver_Count(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	// Create a SearchResolver instance with a mock connection pool.
	val1 := "Pod"

	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}}}
	resolver, mockPool := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Mock the database query
	mockRow := &Row{MockValue: 10}
	mockPool.EXPECT().QueryRow(gomock.Any(),
		gomock.Eq(`SELECT COUNT("uid") FROM "search"."resources" WHERE ("data"->'kind'?('Pod') AND ("cluster" = ANY ('{}')))`),
		gomock.Eq([]interface{}{})).Return(mockRow)

	// Execute function
	r, err := resolver.Count()
	assert.Nil(t, err)

	// Verify response
	if r != mockRow.MockValue {
		t.Errorf("Incorrect Count() expected [%d] got [%d]", mockRow.MockValue, r)
	}
}

func Test_SearchResolver_MatchManagedHubCount(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string", "managedHub": "string"}
	// Create a SearchResolver instance with a mock connection pool.
	config.Cfg.HubName = "test-hub-a"
	val1 := "Pod"
	managedHub := "test-hub-a"

	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}, {Property: "managedHub", Values: []*string{&managedHub}}}}
	resolver, mockPool := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Mock the database query
	mockRow := &Row{MockValue: 10}
	mockPool.EXPECT().QueryRow(gomock.Any(),
		gomock.Eq(`SELECT COUNT("uid") FROM "search"."resources" WHERE ("data"->'kind'?('Pod') AND ("cluster" = ANY ('{}')))`),
		gomock.Eq([]interface{}{})).Return(mockRow)

	// Execute function
	r, err := resolver.Count()
	assert.Nil(t, err)

	// Verify response
	if r != mockRow.MockValue {
		t.Errorf("Incorrect Count() expected [%d] got [%d]", mockRow.MockValue, r)
	}
}

func Test_SearchResolver_NotMatchManagedHub(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string", "managedHub": "string"}
	// Create a SearchResolver instance with a mock connection pool.
	config.Cfg.HubName = "test-hub-b"

	val1 := "Pod"
	managedHub := "test-hub-a"
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}, {Property: "managedHub", Values: []*string{&managedHub}}}}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Execute function
	r, err := resolver.Count()
	assert.Nil(t, err)

	// Verify response
	if r != 0 {
		t.Errorf("Incorrect Count() expected [%d] got [%d]", 0, r)
	}
	rItems, err := resolver.Items()
	assert.Nil(t, err)
	assert.Equal(t, len(rItems), 0, "Items() received incorrect number of items. Expected %d Got: %d", 0, len(rItems))

	rRelated, err := resolver.Related(context.TODO())
	assert.Nil(t, err)
	assert.Equal(t, len(rRelated), 0, "Related() received incorrect number of items. Expected %d Got: %d", 0, len(rRelated))

}

func Test_SearchResolver_Count_WithRBAC(t *testing.T) {
	csRes, nsRes, managedClusters := newUserData()
	ud := rbac.UserData{CsResources: csRes, NsResources: nsRes, ManagedClusters: managedClusters}
	// Create a SearchResolver instance with a mock connection pool.
	val1 := "Pod"
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}}}
	propTypesMock := map[string]string{"kind": "string"}
	resolver, mockPool := newMockSearchResolver(t, searchInput, nil, ud, propTypesMock)

	// Mock the database query
	mockRow := &Row{MockValue: 10}
	mockPool.EXPECT().QueryRow(gomock.Any(),
		gomock.Eq(`SELECT COUNT("uid") FROM "search"."resources" WHERE ("data"->'kind'?('Pod') AND (("cluster" = ANY ('{"managed1","managed2"}')) OR ("data"?'_hubClusterResource' AND ((NOT("data"?'namespace') AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'nodes') OR (data->'apigroup'?'storage.k8s.io' AND data->'kind_plural'?'csinodes'))) OR ((data->'namespace'?|'{"default"}' AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'configmaps') OR (data->'apigroup'?'v4' AND data->'kind_plural'?'services'))) OR (data->'namespace'?|'{"ocm"}' AND ((data->'apigroup'?'v1' AND data->'kind_plural'?'pods') OR (data->'apigroup'?'v2' AND data->'kind_plural'?'deployments'))))))))`),
		gomock.Eq([]interface{}{})).Return(mockRow)

	// Execute function
	r, err := resolver.Count()
	assert.Nil(t, err)

	// Verify response
	if r != mockRow.MockValue {
		t.Errorf("Incorrect Count() expected [%d] got [%d]", mockRow.MockValue, r)
	}
}

func Test_SearchResolver_CountWithOperator(t *testing.T) {
	// Create a SearchResolver instance with a mock connection pool.
	val1 := ">=1"
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "current", Values: []*string{&val1}}}}
	ud := rbac.UserData{CsResources: []rbac.Resource{}}
	propTypesMock := map[string]string{"current": "number"}
	resolver, mockPool := newMockSearchResolver(t, searchInput, nil, ud, propTypesMock)

	// Mock the database query
	mockRow := &Row{MockValue: 1}
	mockPool.EXPECT().QueryRow(gomock.Any(),
		gomock.Eq(`SELECT COUNT("uid") FROM "search"."resources" WHERE ((("data"->'current')::numeric >= '1') AND ("cluster" = ANY ('{}')))`),
		gomock.Eq([]interface{}{})).Return(mockRow)

	// Execute function
	r, err := resolver.Count()
	assert.Nil(t, err)
	// Verify response
	if r != mockRow.MockValue {
		t.Errorf("Incorrect Count() expected [%d] got [%d]", mockRow.MockValue, r)
	}
}

func Test_SearchResolver_CountWithOperatorNum(t *testing.T) {
	// Create a SearchResolver instance with a mock connection pool.
	val1 := "1"
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "current", Values: []*string{&val1}}}}
	ud := rbac.UserData{CsResources: []rbac.Resource{}}
	propTypesMock := map[string]string{"kind": "string", "current": "number"}
	resolver, mockPool := newMockSearchResolver(t, searchInput, nil, ud, propTypesMock)

	// Mock the database query
	mockRow := &Row{MockValue: 1}
	mockPool.EXPECT().QueryRow(gomock.Any(),
		gomock.Eq(`SELECT COUNT("uid") FROM "search"."resources" WHERE ((("data"->'current')::numeric IN ('1')) AND ("cluster" = ANY ('{}')))`),
		gomock.Eq([]interface{}{})).Return(mockRow)

	// Execute function
	r, err := resolver.Count()
	assert.Nil(t, err)
	// Verify response
	if r != mockRow.MockValue {
		t.Errorf("Incorrect Count(): expected [%d] got [%d]", mockRow.MockValue, r)
	}
}

func Test_SearchResolver_CountWithOperatorString(t *testing.T) {
	// Create a SearchResolver instance with a mock connection pool.
	val1 := "=Template"
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}}}
	ud := rbac.UserData{CsResources: []rbac.Resource{}}
	propTypesMock := map[string]string{"kind": "string"}
	resolver, mockPool := newMockSearchResolver(t, searchInput, nil, ud, propTypesMock)

	// Mock the database query
	mockRow := &Row{MockValue: 1}
	mockPool.EXPECT().QueryRow(gomock.Any(),
		gomock.Eq(`SELECT COUNT("uid") FROM "search"."resources" WHERE ("data"->'kind'?('Template') AND ("cluster" = ANY ('{}')))`),
		gomock.Eq([]interface{}{})).Return(mockRow)

	// Execute function
	r, err := resolver.Count()
	assert.Nil(t, err)
	// Verify response
	if r != mockRow.MockValue {
		t.Errorf("Incorrect Count() expected [%d] got [%d]", mockRow.MockValue, r)
	}
}

func Test_SearchResolver_Items(t *testing.T) {
	// Create a SearchResolver instance with a mock connection pool.
	val1 := "template"
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}}}
	propTypesMock := map[string]string{"kind": "string"}

	ud := rbac.UserData{CsResources: []rbac.Resource{}}

	resolver, mockPool := newMockSearchResolver(t, searchInput, nil, ud, propTypesMock)
	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("./mocks/mock.json", searchInput, "string", 0)

	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE (("data"->>'kind' ILIKE ANY ('{"template"}')) AND ("cluster" = ANY ('{}'))) LIMIT 1000`),
		gomock.Eq([]interface{}{}),
	).Return(mockRows, nil)

	// Execute the function
	result, err := resolver.Items()
	assert.Nil(t, err)

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

func Test_SearchResolver_ItemsWithNumOperator(t *testing.T) {
	val1 := ">1"
	testOperatorGreater := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: "current", Values: []*string{&val1}}}},
		mockQuery:   `SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE ((("data"->'current')::numeric > '1') AND (("cluster" = ANY ('{"managed1","managed2"}')) OR ("data"?'_hubClusterResource' AND ((NOT("data"?'namespace') AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'nodes') OR (data->'apigroup'?'storage.k8s.io' AND data->'kind_plural'?'csinodes'))) OR ((data->'namespace'?|'{"default"}' AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'configmaps') OR (data->'apigroup'?'v4' AND data->'kind_plural'?'services'))) OR (data->'namespace'?|'{"ocm"}' AND ((data->'apigroup'?'v1' AND data->'kind_plural'?'pods') OR (data->'apigroup'?'v2' AND data->'kind_plural'?'deployments')))))))) LIMIT 1000`,
	}
	val2 := "<4"
	testOperatorLesser := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: "current", Values: []*string{&val2}}}},
		mockQuery:   `SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE ((("data"->'current')::numeric < '4') AND (("cluster" = ANY ('{"managed1","managed2"}')) OR ("data"?'_hubClusterResource' AND ((NOT("data"?'namespace') AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'nodes') OR (data->'apigroup'?'storage.k8s.io' AND data->'kind_plural'?'csinodes'))) OR ((data->'namespace'?|'{"default"}' AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'configmaps') OR (data->'apigroup'?'v4' AND data->'kind_plural'?'services'))) OR (data->'namespace'?|'{"ocm"}' AND ((data->'apigroup'?'v1' AND data->'kind_plural'?'pods') OR (data->'apigroup'?'v2' AND data->'kind_plural'?'deployments')))))))) LIMIT 1000`,
	}
	val3 := ">=1"
	testOperatorGreaterorEqual := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: "current", Values: []*string{&val3}}}},
		mockQuery:   `SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE ((("data"->'current')::numeric >= '1') AND (("cluster" = ANY ('{"managed1","managed2"}')) OR ("data"?'_hubClusterResource' AND ((NOT("data"?'namespace') AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'nodes') OR (data->'apigroup'?'storage.k8s.io' AND data->'kind_plural'?'csinodes'))) OR ((data->'namespace'?|'{"default"}' AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'configmaps') OR (data->'apigroup'?'v4' AND data->'kind_plural'?'services'))) OR (data->'namespace'?|'{"ocm"}' AND ((data->'apigroup'?'v1' AND data->'kind_plural'?'pods') OR (data->'apigroup'?'v2' AND data->'kind_plural'?'deployments')))))))) LIMIT 1000`,
	}
	val4 := "<=3"
	testOperatorLesserorEqual := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: "current", Values: []*string{&val4}}}},
		mockQuery:   `SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE ((("data"->'current')::numeric <= '3') AND (("cluster" = ANY ('{"managed1","managed2"}')) OR ("data"?'_hubClusterResource' AND ((NOT("data"?'namespace') AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'nodes') OR (data->'apigroup'?'storage.k8s.io' AND data->'kind_plural'?'csinodes'))) OR ((data->'namespace'?|'{"default"}' AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'configmaps') OR (data->'apigroup'?'v4' AND data->'kind_plural'?'services'))) OR (data->'namespace'?|'{"ocm"}' AND ((data->'apigroup'?'v1' AND data->'kind_plural'?'pods') OR (data->'apigroup'?'v2' AND data->'kind_plural'?'deployments')))))))) LIMIT 1000`,
	}

	val5 := "!4"
	testOperatorNot := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: "current", Values: []*string{&val5}}}},
		mockQuery:   `SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE ((("data"->'current')::numeric NOT IN ('4')) AND (("cluster" = ANY ('{"managed1","managed2"}')) OR ("data"?'_hubClusterResource' AND ((NOT("data"?'namespace') AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'nodes') OR (data->'apigroup'?'storage.k8s.io' AND data->'kind_plural'?'csinodes'))) OR ((data->'namespace'?|'{"default"}' AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'configmaps') OR (data->'apigroup'?'v4' AND data->'kind_plural'?'services'))) OR (data->'namespace'?|'{"ocm"}' AND ((data->'apigroup'?'v1' AND data->'kind_plural'?'pods') OR (data->'apigroup'?'v2' AND data->'kind_plural'?'deployments')))))))) LIMIT 1000`,
	}

	val6 := "!=4"
	testOperatorNotEqual := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: "current", Values: []*string{&val6}}}},
		mockQuery:   `SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE ((("data"->'current')::numeric NOT IN ('4')) AND (("cluster" = ANY ('{"managed1","managed2"}')) OR ("data"?'_hubClusterResource' AND ((NOT("data"?'namespace') AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'nodes') OR (data->'apigroup'?'storage.k8s.io' AND data->'kind_plural'?'csinodes'))) OR ((data->'namespace'?|'{"default"}' AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'configmaps') OR (data->'apigroup'?'v4' AND data->'kind_plural'?'services'))) OR (data->'namespace'?|'{"ocm"}' AND ((data->'apigroup'?'v1' AND data->'kind_plural'?'pods') OR (data->'apigroup'?'v2' AND data->'kind_plural'?'deployments')))))))) LIMIT 1000`,
	}

	val7 := "=3"
	testOperatorEqual := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: "current", Values: []*string{&val7}}}},
		mockQuery:   `SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE ((("data"->'current')::numeric IN ('3')) AND (("cluster" = ANY ('{"managed1","managed2"}')) OR ("data"?'_hubClusterResource' AND ((NOT("data"?'namespace') AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'nodes') OR (data->'apigroup'?'storage.k8s.io' AND data->'kind_plural'?'csinodes'))) OR ((data->'namespace'?|'{"default"}' AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'configmaps') OR (data->'apigroup'?'v4' AND data->'kind_plural'?'services'))) OR (data->'namespace'?|'{"ocm"}' AND ((data->'apigroup'?'v1' AND data->'kind_plural'?'pods') OR (data->'apigroup'?'v2' AND data->'kind_plural'?'deployments')))))))) LIMIT 1000`,
	}

	testOperatorMultiple := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: "current", Values: []*string{&val1, &val2}}}},
		mockQuery:   `SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE (((("data"->'current')::numeric < '4') OR (("data"->'current')::numeric > '1')) AND (("cluster" = ANY ('{"managed1","managed2"}')) OR ("data"?'_hubClusterResource' AND ((NOT("data"?'namespace') AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'nodes') OR (data->'apigroup'?'storage.k8s.io' AND data->'kind_plural'?'csinodes'))) OR ((data->'namespace'?|'{"default"}' AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'configmaps') OR (data->'apigroup'?'v4' AND data->'kind_plural'?'services'))) OR (data->'namespace'?|'{"ocm"}' AND ((data->'apigroup'?'v1' AND data->'kind_plural'?'pods') OR (data->'apigroup'?'v2' AND data->'kind_plural'?'deployments')))))))) LIMIT 1000`,
	}

	testOperators := []TestOperatorItem{
		testOperatorGreater, testOperatorLesser, testOperatorGreaterorEqual,
		testOperatorLesserorEqual, testOperatorNot, testOperatorNotEqual, testOperatorEqual,
		testOperatorMultiple,
	}
	testAllOperators(t, testOperators)
}
func Test_SearchResolver_ItemsWithDateOperator(t *testing.T) {
	//define schema table:
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)
	prop := "created"

	val8 := "year"
	opValMap := getOperatorIfDateFilter(prop, []string{val8}, map[string][]string{})
	csres, nsres, mc := newUserData()

	rbac := buildRbacWhereClause(context.TODO(),
		rbac.UserData{CsResources: csres, NsResources: nsres, ManagedClusters: mc},
		getUserInfo())
	mockQueryYear, _, _ := ds.SelectDistinct("uid", "cluster", "data").Where(goqu.L(`"data"->>?`, prop).Gt(opValMap[">"][0]), rbac).Limit(1000).ToSQL()

	testOperatorYear := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: prop, Values: []*string{&val8}}}},
		mockQuery:   mockQueryYear, // `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->>'created' > ('2021-05-16T13:11:12Z')) LIMIT 1000`,
	}

	val9 := "hour"
	opValMap = getOperatorIfDateFilter(prop, []string{val9}, map[string][]string{})
	mockQueryHour, _, _ := ds.SelectDistinct("uid", "cluster", "data").Where(goqu.L(`"data"->>?`, prop).Gt(opValMap[">"][0]), rbac).Limit(1000).ToSQL()

	testOperatorHour := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: prop, Values: []*string{&val9}}}},
		mockQuery:   mockQueryHour, // `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->>'created' > ('2021-05-16T13:11:12Z')) LIMIT 1000`,
	}

	val10 := "day"
	opValMap = getOperatorIfDateFilter(prop, []string{val10}, map[string][]string{})
	mockQueryDay, _, _ := ds.SelectDistinct("uid", "cluster", "data").Where(goqu.L(`"data"->>?`, prop).Gt(goqu.L("?", opValMap[">"][0])), rbac).Limit(1000).ToSQL()

	testOperatorDay := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: prop, Values: []*string{&val10}}}},
		mockQuery:   mockQueryDay, // `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->>'created' > ('2021-05-16T13:11:12Z')) LIMIT 1000`,
	}

	val11 := "week"
	opValMap = getOperatorIfDateFilter(prop, []string{val11}, map[string][]string{})
	mockQueryWeek, _, _ := ds.SelectDistinct("uid", "cluster", "data").Where(goqu.L(`"data"->>?`, prop).Gt(goqu.L("?", opValMap[">"][0])), rbac).Limit(1000).ToSQL()

	testOperatorWeek := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: prop, Values: []*string{&val11}}}},
		mockQuery:   mockQueryWeek, // `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->>'created' > ('2021-05-16T13:11:12Z')) LIMIT 1000`,
	}

	val12 := "month"
	opValMap = getOperatorIfDateFilter(prop, []string{val12}, map[string][]string{})
	mockQueryMonth, _, _ := ds.SelectDistinct("uid", "cluster", "data").Where(goqu.L(`"data"->>?`, prop).Gt(goqu.L("?", opValMap[">"][0])), rbac).Limit(1000).ToSQL()

	testOperatorMonth := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: prop, Values: []*string{&val12}}}},
		mockQuery:   mockQueryMonth, // `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->>'created' > ('2021-05-16T13:11:12Z')) LIMIT 1000`,
	}
	opValMap = getOperatorIfDateFilter(prop, []string{val8, val9}, map[string][]string{})
	mockQueryMultiple, _, _ := ds.SelectDistinct("uid", "cluster", "data").Where(goqu.Or(goqu.L(`"data"->>?`, prop).Gt(opValMap[">"][0]),
		goqu.L(`"data"->>?`, prop).Gt(opValMap[">"][1])), rbac).Limit(1000).ToSQL()

	testoperatorMultiple := TestOperatorItem{
		searchInput: &model.SearchInput{Filters: []*model.SearchFilter{{Property: prop, Values: []*string{&val8, &val9}}}},
		mockQuery:   mockQueryMultiple, // `SELECT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->>'created' > ('2021-05-16T13:11:12Z')) LIMIT 1000`,
	}
	testOperators := []TestOperatorItem{
		testOperatorYear, testOperatorHour, testOperatorDay, testOperatorWeek, testOperatorMonth,
		testoperatorMultiple,
	}
	testAllOperators(t, testOperators)

}

func testAllOperators(t *testing.T, testOperators []TestOperatorItem) {
	for _, currTest := range testOperators {
		csRes, nsRes, mc := newUserData()
		ud := rbac.UserData{CsResources: csRes, NsResources: nsRes, ManagedClusters: mc}
		propTypesMock := map[string]string{"current": "number", "created": "timestamp"}
		// Create a SearchResolver instance with a mock connection pool.
		resolver, mockPool := newMockSearchResolver(t, currTest.searchInput, nil, ud, propTypesMock)
		// Mock the database queries.
		mockRows := newMockRowsWithoutRBAC("./mocks/mock.json", currTest.searchInput, "number", 0)

		mockPool.EXPECT().Query(gomock.Any(),
			gomock.Eq(currTest.mockQuery),
			gomock.Eq([]interface{}{}),
		).Return(mockRows, nil)

		// Execute the function
		result, err := resolver.Items()
		assert.Nil(t, err)
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
	ud := rbac.UserData{CsResources: []rbac.Resource{}}
	propTypesMock := map[string]string{"cluster": "string", "namespace": "string"}

	resolver, mockPool := newMockSearchResolver(t, searchInput, nil, ud, propTypesMock)

	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("./mocks/mock.json", searchInput, "string", 0)
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->'namespace'?|'{"openshift","openshift-monitoring"}' AND ("cluster" IN ('local-cluster')) AND ("cluster" = ANY ('{}'))) LIMIT 10`),
		// gomock.Eq("SELECT uid, cluster, data FROM search.resources  WHERE lower(data->> 'namespace')=any($1) AND cluster=$2 LIMIT 10"),
		gomock.Eq([]interface{}{}),
	).Return(mockRows, nil)

	// Execute the function
	result, err := resolver.Items()
	assert.Nil(t, err)

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
	ud := rbac.UserData{CsResources: []rbac.Resource{}}
	propTypesMock := map[string]string{"namespace": "string", "cluster": "string"}

	resolver, mockPool := newMockSearchResolver(t, searchInput, nil, ud, propTypesMock)

	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("../resolver/mocks/mock.json", searchInput, "string", 0)

	// Mock the database query
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->'namespace'?('openshift') AND ("cluster" IN ('local-cluster', 'remote-1')) AND ("cluster" = ANY ('{}')))`),
		gomock.Eq([]interface{}{})).Return(mockRows, nil)

	// Execute function
	result, err := resolver.Items()
	assert.Nil(t, err)

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

func Test_SearchResolver_Keywords(t *testing.T) {
	// Create a SearchResolver instance with a mock connection pool.
	val1 := "Template"
	limit := 10
	searchInput := &model.SearchInput{Keywords: []*string{&val1}, Limit: &limit}
	ud := rbac.UserData{CsResources: []rbac.Resource{}}
	propTypesMock := map[string]string{"kind": "string"}

	resolver, mockPool := newMockSearchResolver(t, searchInput, nil, ud, propTypesMock)

	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("./mocks/mock.json", searchInput, "string", 0)

	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources", jsonb_each_text("data") WHERE (("value" ILIKE '%Template%') AND ("cluster" = ANY ('{}'))) LIMIT 10`),
		gomock.Eq([]interface{}{}),
	).Return(mockRows, nil)

	// Execute the function
	result, err := resolver.Items()
	assert.Nil(t, err)

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

func Test_SearchResolver_Uids(t *testing.T) {
	// Create a SearchResolver instance with a mock connection pool.
	val1 := "template"
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}}}
	propTypesMock := map[string]string{"kind": "string"}

	resolver, mockPool := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)
	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("./mocks/mock.json", searchInput, "string", 0)

	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT "uid" FROM "search"."resources" WHERE (("data"->>'kind' ILIKE ANY ('{"template"}')) AND ("cluster" = ANY ('{}'))) LIMIT 1000`),
		gomock.Eq([]interface{}{}),
	).Return(mockRows, nil)

	// Execute the function
	err := resolver.Uids()
	assert.Nil(t, err)

	// Verify returned items.
	if len(resolver.uids) != len(mockRows.mockData) {
		t.Errorf("Items() received incorrect number of items. Expected %d Got: %d", len(mockRows.mockData), len(resolver.uids))
	}

	// Verify properties for each returned item.
	for i, item := range resolver.uids {
		mockRow := mockRows.mockData[i]
		expectedRow := formatDataMap(mockRow["data"].(map[string]interface{}))
		expectedRow["_uid"] = mockRow["uid"]

		if *item != mockRow["uid"].(string) {
			t.Errorf("Value of key [uid] does not match for item [%d].\nExpected: %s\nGot: %s", i, expectedRow["_uid"], *item)
		}
	}
}

func Test_buildRbacWhereClauseCs(t *testing.T) {
	csres, _, _ := newUserData()
	ud := rbac.UserData{CsResources: csres}

	rbacCombined := buildRbacWhereClause(context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456"),
		ud, getUserInfo())
	expectedSql := `SELECT * WHERE (("cluster" = ANY ('{}')) OR ("data"?'_hubClusterResource' AND (NOT("data"?'namespace') AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'nodes') OR (data->'apigroup'?'storage.k8s.io' AND data->'kind_plural'?'csinodes')))))`
	gotSql, _, _ := goqu.Select().Where(rbacCombined).ToSQL()
	assert.Equal(t, expectedSql, gotSql)
}

func Test_buildRbacWhereClauseNs(t *testing.T) {
	_, nsScopeAccess, _ := newUserData()
	ud := rbac.UserData{NsResources: nsScopeAccess}

	rbacCombined := buildRbacWhereClause(context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456"),
		ud, getUserInfo())
	expectedSql := `SELECT * WHERE (("cluster" = ANY ('{}')) OR ("data"?'_hubClusterResource' AND ((data->'namespace'?|'{"default"}' AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'configmaps') OR (data->'apigroup'?'v4' AND data->'kind_plural'?'services'))) OR (data->'namespace'?|'{"ocm"}' AND ((data->'apigroup'?'v1' AND data->'kind_plural'?'pods') OR (data->'apigroup'?'v2' AND data->'kind_plural'?'deployments'))))))`
	gotSql, _, _ := goqu.Select().Where(rbacCombined).ToSQL()
	assert.Equal(t, expectedSql, gotSql)

}

func Test_buildRbacWhereClauseCsAndNs(t *testing.T) {
	res, nsScopeAccess, _ := newUserData()
	ud := rbac.UserData{CsResources: res, NsResources: nsScopeAccess}
	rbacCombined := buildRbacWhereClause(context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456"),
		ud, getUserInfo())
	expectedSql := `SELECT * WHERE (("cluster" = ANY ('{}')) OR ("data"?'_hubClusterResource' AND ((NOT("data"?'namespace') AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'nodes') OR (data->'apigroup'?'storage.k8s.io' AND data->'kind_plural'?'csinodes'))) OR ((data->'namespace'?|'{"default"}' AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'configmaps') OR (data->'apigroup'?'v4' AND data->'kind_plural'?'services'))) OR (data->'namespace'?|'{"ocm"}' AND ((data->'apigroup'?'v1' AND data->'kind_plural'?'pods') OR (data->'apigroup'?'v2' AND data->'kind_plural'?'deployments')))))))`
	gotSql, _, _ := goqu.Select().Where(rbacCombined).ToSQL()
	assert.Equal(t, expectedSql, gotSql)

}

func Test_buildRbacWhereClauseCsNsAndMc(t *testing.T) {
	csres, nsScopeAccess, managedClusters := newUserData()
	ud := rbac.UserData{CsResources: csres, NsResources: nsScopeAccess, ManagedClusters: managedClusters}
	rbacCombined := buildRbacWhereClause(context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456"),
		ud, getUserInfo())
	expectedSql := `SELECT * WHERE (("cluster" = ANY ('{"managed1","managed2"}')) OR ("data"?'_hubClusterResource' AND ((NOT("data"?'namespace') AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'nodes') OR (data->'apigroup'?'storage.k8s.io' AND data->'kind_plural'?'csinodes'))) OR ((data->'namespace'?|'{"default"}' AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'configmaps') OR (data->'apigroup'?'v4' AND data->'kind_plural'?'services'))) OR (data->'namespace'?|'{"ocm"}' AND ((data->'apigroup'?'v1' AND data->'kind_plural'?'pods') OR (data->'apigroup'?'v2' AND data->'kind_plural'?'deployments')))))))`
	gotSql, _, _ := goqu.Select().Where(rbacCombined).ToSQL()
	assert.Equal(t, expectedSql, gotSql)
}

func Test_SearchResolver_Items_Labels(t *testing.T) {
	// Create a SearchResolver instance with a mock connection pool.
	cluster := "local-cluster"
	val1 := "Template"

	val2 := "samples.operator.openshift.io/managed=true"
	limit := 10
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}, {Property: "cluster", Values: []*string{&cluster}}, {Property: "label", Values: []*string{&val2}}}, Limit: &limit}
	ud := rbac.UserData{CsResources: []rbac.Resource{}}
	propTypesMock := map[string]string{"cluster": "string", "kind": "string", "label": "object"}

	resolver, mockPool := newMockSearchResolver(t, searchInput, nil, ud, propTypesMock)

	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("./mocks/mock.json", searchInput, "string", limit)

	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->'kind'?('Template') AND ("cluster" IN ('local-cluster')) AND "data"->'label' @> '{"samples.operator.openshift.io/managed":"true"}' AND ("cluster" = ANY ('{}'))) LIMIT 10`),
		gomock.Eq([]interface{}{}),
	).Return(mockRows, nil)

	// Execute the function
	result, err := resolver.Items()
	assert.Nil(t, err)

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

func Test_SearchResolver_Items_Container(t *testing.T) {
	// Create a SearchResolver instance with a mock connection pool.
	cluster := "local-cluster"
	val1 := "Template"
	val2 := "acm-agent"
	limit := 10
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}, {Property: "cluster", Values: []*string{&cluster}}, {Property: "container", Values: []*string{&val2}}}, Limit: &limit}
	ud := rbac.UserData{CsResources: []rbac.Resource{}}
	propTypesMock := map[string]string{"cluster": "string", "kind": "string", "container": "array"}

	resolver, mockPool := newMockSearchResolver(t, searchInput, nil, ud, propTypesMock)

	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("./mocks/mock.json", searchInput, "array", limit)

	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->'kind'?('Template') AND ("cluster" IN ('local-cluster')) AND "data"->'container' @> '["acm-agent"]' AND ("cluster" = ANY ('{}'))) LIMIT 10`),
		gomock.Eq([]interface{}{}),
	).Return(mockRows, nil)

	// Execute the function
	result, err := resolver.Items()
	assert.Nil(t, err)

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
func Test_buildRbacWhereClauseHandleStars(t *testing.T) {
	ud := rbac.UserData{
		CsResources:     []rbac.Resource{{Apigroup: "", Kind: "nodes"}, {Apigroup: "*", Kind: "csinodes"}},
		NsResources:     map[string][]rbac.Resource{"ocm": {{Apigroup: "*", Kind: "pods"}, {Apigroup: "*", Kind: "deployments"}}},
		ManagedClusters: map[string]struct{}{"managed1": {}, "managed2": {}}}
	rbacCombined := buildRbacWhereClause(context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456"),
		ud, getUserInfo())
	expectedSql := `SELECT * WHERE (("cluster" = ANY ('{"managed1","managed2"}')) OR ("data"?'_hubClusterResource' AND ((NOT("data"?'namespace') AND ((NOT("data"?'apigroup') AND data->'kind_plural'?'nodes') OR data->'kind_plural'?'csinodes')) OR (data->'namespace'?|'{"ocm"}' AND (data->'kind_plural'?'pods' OR data->'kind_plural'?'deployments')))))`
	gotSql, _, _ := goqu.Select().Where(rbacCombined).ToSQL()
	assert.Equal(t, expectedSql, gotSql)
}

func Test_buildRbacWhereClauseHandleAllStars(t *testing.T) {
	ud := rbac.UserData{
		CsResources:     []rbac.Resource{{Apigroup: "", Kind: "nodes"}, {Apigroup: "*", Kind: "*"}},
		NsResources:     map[string][]rbac.Resource{"ocm": {{Apigroup: "*", Kind: "pods"}, {Apigroup: "*", Kind: "*"}}},
		ManagedClusters: map[string]struct{}{"managed1": {}, "managed2": {}}}
	rbacCombined := buildRbacWhereClause(context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456"),
		ud, getUserInfo())
	expectedSql := `SELECT * WHERE (("cluster" = ANY ('{"managed1","managed2"}')) OR ("data"?'_hubClusterResource' AND (NOT("data"?'namespace') OR data->'namespace'?|'{"ocm"}')))`
	gotSql, _, _ := goqu.Select().Where(rbacCombined).ToSQL()
	assert.Equal(t, expectedSql, gotSql)
}

func Test_SearchResolver_UidsAllAccess(t *testing.T) {
	// Create a SearchResolver instance with a mock connection pool.
	val1 := "template"
	propTypesMock := map[string]string{"kind": "string"}
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}}}
	resolver, mockPool := newMockSearchResolver(t, searchInput, nil, rbac.UserData{
		CsResources:     []rbac.Resource{{Apigroup: "*", Kind: "*"}},
		NsResources:     map[string][]rbac.Resource{"*": {{Apigroup: "*", Kind: "*"}}},
		ManagedClusters: map[string]struct{}{"managed-cluster1": {}},
	},
		propTypesMock)
	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("./mocks/mock.json", searchInput, "string", 0)

	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT "uid" FROM "search"."resources" WHERE (("data"->>'kind' ILIKE ANY ('{"template"}')) AND (("cluster" = ANY ('{"managed-cluster1"}')) OR "data"?'_hubClusterResource')) LIMIT 1000`),
		gomock.Eq([]interface{}{}),
	).Return(mockRows, nil)

	// Execute the function
	err := resolver.Uids()
	assert.Nil(t, err)

	// Verify returned items.
	if len(resolver.uids) != len(mockRows.mockData) {
		t.Errorf("Items() received incorrect number of items. Expected %d Got: %d", len(mockRows.mockData), len(resolver.uids))
	}

	// Verify properties for each returned item.
	for i, item := range resolver.uids {
		mockRow := mockRows.mockData[i]
		expectedRow := formatDataMap(mockRow["data"].(map[string]interface{}))
		expectedRow["_uid"] = mockRow["uid"]

		if *item != mockRow["uid"].(string) {
			t.Errorf("Value of key [uid] does not match for item [%d].\nExpected: %s\nGot: %s", i, expectedRow["_uid"], *item)
		}
	}
}

func Test_checkErrorBuildingQuery(t *testing.T) {
	mock := SearchResult{query: "Mock query", params: []interface{}{}}

	mock.checkErrorBuildingQuery(fmt.Errorf("mock error"), "Mock error message")

	assert.Equal(t, mock.query, "", "Query should be cleared after error")
	assert.Nil(t, mock.params)
}

func Test_whereClauseFilter_IgnoreNoValues(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val := "Pod"
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "prop1", Values: []*string{}},
		{Property: "kind", Values: []*string{&val}}}}
	// execute function - this should ignore the first filter as it has no values
	whereDs, propTypes, err := WhereClauseFilter(context.TODO(), searchInput, propTypesMock)

	assert.Equal(t, len(whereDs), 1, "whereDs should have 1 expression")
	assert.Equal(t, propTypes, propTypesMock, "propTypes should have only kind property")
	assert.Nil(t, err)
}

func Test_buildSearchQuery_EmptyQueryWithoutRbac(t *testing.T) {

	// Create a SearchResolver instance with a mock connection pool.
	val1 := "template"
	propTypesMock := map[string]string{"kind": "string"}
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}}}
	resolver, mockPool := newMockSearchResolver(t, searchInput, nil, rbac.UserData{}, //user data is nil
		propTypesMock)
	// Mock the database queries.
	mockPool.EXPECT().QueryRow(gomock.Any(),
		gomock.Eq(``), //query will be empty as user data for rbac is not provided
		gomock.Eq([]interface{}{}),
	).Return(&Row{MockValue: 0})
	// This should become empty after function execution
	resolver.query = "mock Query"
	// execute function
	_, err := resolver.Count()
	if !strings.Contains(err.Error(), "RBAC clause is required!") {
		t.Errorf("Expected error %s. but got %s", "RBAC clause is required! None found for search query", err.Error())
	}
	assert.Equal(t, resolver.query, "", "query should be empty as there is no rbac clause")
}

func Test_buildSearchQuery_EmptyQueryNoFilter(t *testing.T) {

	// Create a SearchResolver instance with a mock connection pool.
	propTypesMock := map[string]string{"kind": "string"}
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{}}
	resolver, mockPool := newMockSearchResolver(t, searchInput, nil, rbac.UserData{},
		propTypesMock)
	// Mock the database queries.
	mockPool.EXPECT().QueryRow(gomock.Any(),
		gomock.Eq(``), //query will be empty as user data for rbac is not provided
		gomock.Eq([]interface{}{}),
	).Return(&Row{MockValue: 0})
	// This should become empty after function execution
	resolver.query = "mock Query"
	// execute function
	_, err := resolver.Count()
	if !strings.Contains(err.Error(), "query input must contain a filter or keyword") {
		t.Errorf("Expected error %s. but got %s", "query input must contain a filter or keyword", err.Error())
	}
	assert.Equal(t, resolver.query, "", "query should be empty as search filter is not provided")
}

func Test_SearchResolver_SearchUserAllAccess(t *testing.T) {
	// Create a SearchResolver instance with a mock connection pool.
	val1 := "template"
	propTypesMock := map[string]string{"kind": "string"}
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}}}
	resolver, mockPool := newMockSearchResolver(t, searchInput, nil, rbac.UserData{
		CsResources:     []rbac.Resource{},
		NsResources:     map[string][]rbac.Resource{},
		ManagedClusters: map[string]struct{}{"*": {}},
	},
		propTypesMock)
	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("./mocks/mock.json", searchInput, "string", 0)

	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT "uid" FROM "search"."resources" WHERE (("data"->>'kind' ILIKE ANY ('{"template"}')) AND ("cluster" NOT IN (SELECT "cluster" FROM "search"."resources" WHERE ((data ? '_hubClusterResource') IS TRUE) LIMIT 1))) LIMIT 1000`),
		gomock.Eq([]interface{}{}),
	).Return(mockRows, nil)

	// Execute the function
	err := resolver.Uids()
	assert.Nil(t, err)

	// Verify returned items.
	if len(resolver.uids) != len(mockRows.mockData) {
		t.Errorf("Items() received incorrect number of items. Expected %d Got: %d", len(mockRows.mockData), len(resolver.uids))
	}

	// Verify properties for each returned item.
	for i, item := range resolver.uids {
		mockRow := mockRows.mockData[i]
		expectedRow := formatDataMap(mockRow["data"].(map[string]interface{}))
		expectedRow["_uid"] = mockRow["uid"]

		if *item != mockRow["uid"].(string) {
			t.Errorf("Value of key [uid] does not match for item [%d].\nExpected: %s\nGot: %s", i, expectedRow["_uid"], *item)
		}
	}
}

func Test_SearchResolver_Items_WrongLabelFormat(t *testing.T) {
	// Create a SearchResolver instance with a mock connection pool.
	cluster := "local-cluster"
	val1 := "Template"

	val2 := "samples.operator.openshift.io/managed"
	limit := 10
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}, {Property: "cluster", Values: []*string{&cluster}}, {Property: "label", Values: []*string{&val2}}}, Limit: &limit}
	ud := rbac.UserData{CsResources: []rbac.Resource{}}
	propTypesMock := map[string]string{"cluster": "string", "kind": "string", "label": "object"}

	resolver, _ := newMockSearchResolver(t, searchInput, nil, ud, propTypesMock)

	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("./mocks/mock.json", searchInput, "string", limit)

	// Execute the function
	result, err := resolver.Items()
	assert.Equal(t, "incorrect label format, label filters must have the format key=value", err.Error())
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

func Test_SearchResolver_Items_MultipleLabels(t *testing.T) {
	// Create a SearchResolver instance with a mock connection pool.
	cluster := "local-cluster"
	val1 := "Template"

	val2 := "samples*=tru*"
	val3 := "app*=*prometheus*"
	limit := 10
	searchInput := &model.SearchInput{Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}, {Property: "cluster", Values: []*string{&cluster}}, {Property: "label", Values: []*string{&val2, &val3}}}, Limit: &limit}
	ud := rbac.UserData{CsResources: []rbac.Resource{}}
	propTypesMock := map[string]string{"cluster": "string", "kind": "string", "label": "object"}

	resolver, mockPool := newMockSearchResolver(t, searchInput, nil, ud, propTypesMock)

	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("./mocks/mock.json", searchInput, "string", limit)
	mockPool.EXPECT().Query(gomock.Any(), gomock.Eq(`SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->'kind'?('Template') AND ("cluster" IN ('local-cluster')) AND EXISTS((SELECT 1 FROM jsonb_each_text("data"->'label') As kv(key, value) WHERE (((key LIKE 'samples%') AND (value LIKE 'tru%')) OR ((key LIKE 'app%') AND (value LIKE '%prometheus%'))))) AND ("cluster" = ANY ('{}'))) LIMIT 10`), gomock.Eq([]interface{}{})).Return(mockRows, nil)

	// Execute the function
	result, err := resolver.Items()
	// Verify returned items.
	if len(result) != len(mockRows.mockData) {
		t.Errorf("Items() received incorrect number of items. Expected %d Got: %d", len(mockRows.mockData), len(result))
	}

	assert.Nil(t, err)
	assert.Len(t, result, len(mockRows.mockData))

	for i, item := range result {
		mockRow := mockRows.mockData[i]
		expectedRow := formatDataMap(mockRow["data"].(map[string]interface{}))
		expectedRow["_uid"] = mockRow["uid"]
		expectedRow["cluster"] = mockRow["cluster"]

		assert.Equal(t, expectedRow, item)
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

func TestSearchResolverArrayLabel(t *testing.T) {
	type test struct {
		name          string
		cluster       string
		val1          string
		val2          string
		filterProp1   string
		filterProp2   string
		expectedQuery string
	}

	tests := []test{
		{
			name:          "Match Array",
			cluster:       "local*",
			val1:          "Temp*",
			val2:          "acm-agent",
			filterProp1:   "kind",
			filterProp2:   "container",
			expectedQuery: `SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE (("data"->>'kind' LIKE 'Temp%') AND ("cluster" LIKE 'local%') AND "data"->'container' @> '["acm-agent"]' AND ("cluster" = ANY ('{"test"}'))) LIMIT 10`,
		},
		{
			name:          "Not Match Array",
			cluster:       "local*",
			val1:          "Temp*",
			val2:          `!acm-agent`,
			filterProp1:   "kind",
			filterProp2:   "container",
			expectedQuery: `SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE (("data"->>'kind' LIKE 'Temp%') AND ("cluster" LIKE 'local%') AND NOT("data"->'container' @> '["acm-agent"]') AND ("cluster" = ANY ('{"test"}'))) LIMIT 10`,
		},
		{
			name:          "Not Equal To Match Array",
			cluster:       "local*",
			val1:          "Temp*",
			val2:          `!=acm-agent`,
			filterProp1:   "kind",
			filterProp2:   "container",
			expectedQuery: `SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE (("data"->>'kind' LIKE 'Temp%') AND ("cluster" LIKE 'local%') AND NOT("data"->'container' @> '["acm-agent"]') AND ("cluster" = ANY ('{"test"}'))) LIMIT 10`,
		},
		{
			name:          "Partial Match Array",
			cluster:       "local*",
			val1:          "Temp*",
			val2:          "acm-*",
			filterProp1:   "kind",
			filterProp2:   "container",
			expectedQuery: `SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE (("data"->>'kind' LIKE 'Temp%') AND ("cluster" LIKE 'local%') AND EXISTS((SELECT 1 FROM jsonb_array_elements_text("data"->'container') As arrayProp WHERE (arrayProp LIKE 'acm-%'))) AND ("cluster" = ANY ('{"test"}'))) LIMIT 10`,
		},
		{
			name:          "Partial Not Match Array",
			cluster:       "local*",
			val1:          "Temp*",
			val2:          "!acm-*",
			filterProp1:   "kind",
			filterProp2:   "container",
			expectedQuery: `SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE (("data"->>'kind' LIKE 'Temp%') AND ("cluster" LIKE 'local%') AND NOT EXISTS((SELECT 1 FROM jsonb_array_elements_text("data"->'container') As arrayProp WHERE (arrayProp LIKE 'acm-%'))) AND ("cluster" = ANY ('{"test"}'))) LIMIT 10`,
		},
		{
			name:          "Partial Match Label Key And Value",
			cluster:       "!local*",
			val1:          "Temp*",
			val2:          "samples.operator.openshift.io/man*:tru*",
			filterProp1:   "kind",
			filterProp2:   "label",
			expectedQuery: `SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE (("data"->>'kind' LIKE 'Temp%') AND NOT(("cluster" LIKE 'local%')) AND EXISTS((SELECT 1 FROM jsonb_each_text("data"->'label') As kv(key, value) WHERE ((key LIKE 'samples.operator.openshift.io/man%') AND (value LIKE 'tru%')))) AND ("cluster" = ANY ('{"test"}'))) LIMIT 10`,
		},
		{
			name:          "Partial Match Label Key Or Value",
			cluster:       "!local*",
			val1:          "Temp*",
			val2:          "samples.operator.openshift.io/man*",
			filterProp1:   "kind",
			filterProp2:   "label",
			expectedQuery: `SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE (("data"->>'kind' LIKE 'Temp%') AND NOT(("cluster" LIKE 'local%')) AND EXISTS((SELECT 1 FROM jsonb_each_text("data"->'label') As kv(key, value) WHERE ((key LIKE ('samples.operator.openshift.io/man%')) OR (value LIKE ('samples.operator.openshift.io/man%'))))) AND ("cluster" = ANY ('{"test"}'))) LIMIT 10`,
		},
		{
			name:          "Partial Match Label Not Key Or Value",
			cluster:       "!local*",
			val1:          "Temp*",
			val2:          "!samples.operator.openshift.io/man*=tru*",
			filterProp1:   "kind",
			filterProp2:   "label",
			expectedQuery: `SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE (("data"->>'kind' LIKE 'Temp%') AND NOT(("cluster" LIKE 'local%')) AND NOT EXISTS((SELECT 1 FROM jsonb_each_text("data"->'label') As kv(key, value) WHERE ((key LIKE 'samples.operator.openshift.io/man%') AND (value LIKE 'tru%')))) AND ("cluster" = ANY ('{"test"}'))) LIMIT 10`,
		},
		{
			name:          "Match Label Not Key Or Value",
			cluster:       "!local*",
			val1:          "Temp*",
			val2:          "!samples.operator.openshift.io/managed=true",
			filterProp1:   "kind",
			filterProp2:   "label",
			expectedQuery: `SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE (("data"->>'kind' LIKE 'Temp%') AND NOT(("cluster" LIKE 'local%')) AND NOT("data"->'label' @> '{"samples.operator.openshift.io/managed":"true"}') AND ("cluster" = ANY ('{"test"}'))) LIMIT 10`,
		},
		{
			name:          "Match filter Only star",
			cluster:       "local*",
			val1:          "Temp*",
			val2:          "*",
			filterProp1:   "kind",
			filterProp2:   "namespace",
			expectedQuery: `SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE (("data"->>'kind' LIKE 'Temp%') AND ("cluster" LIKE 'local%') AND ("data"->>'namespace' LIKE '%') AND ("cluster" = ANY ('{"test"}'))) LIMIT 10`,
		},
		{
			name:          "Partial Match 2 Arrays",
			cluster:       "local*",
			val1:          "*agent-1*",
			val2:          "*agent-2*",
			filterProp1:   "container",
			filterProp2:   "container",
			expectedQuery: `SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE (EXISTS((SELECT 1 FROM jsonb_array_elements_text("data"->'container') As arrayProp WHERE (arrayProp LIKE '%agent-1%'))) AND ("cluster" LIKE 'local%') AND EXISTS((SELECT 1 FROM jsonb_array_elements_text("data"->'container') As arrayProp WHERE (arrayProp LIKE '%agent-2%'))) AND ("cluster" = ANY ('{"test"}'))) LIMIT 10`,
		},
		{
			name:          "Match 2 Arrays",
			cluster:       "local-cluster",
			val1:          "acm-agent-1",
			val2:          "acm-agent-2",
			filterProp1:   "container",
			filterProp2:   "container",
			expectedQuery: `SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->'container' @> '["acm-agent-1"]' AND ("cluster" IN ('local-cluster')) AND "data"->'container' @> '["acm-agent-2"]' AND ("cluster" = ANY ('{"test"}'))) LIMIT 10`,
		},
		{
			name:          "Match 1 Arrays And Partial Match 2nd array",
			cluster:       "local-cluster",
			val1:          "acm-agent-1",
			val2:          "*acm-agent-2",
			filterProp1:   "container",
			filterProp2:   "container",
			expectedQuery: `SELECT DISTINCT "uid", "cluster", "data" FROM "search"."resources" WHERE ("data"->'container' @> '["acm-agent-1"]' AND ("cluster" IN ('local-cluster')) AND EXISTS((SELECT 1 FROM jsonb_array_elements_text("data"->'container') As arrayProp WHERE (arrayProp LIKE '%acm-agent-2'))) AND ("cluster" = ANY ('{"test"}'))) LIMIT 10`,
		},
	}

	limit := 10
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			searchInput := &model.SearchInput{
				Filters: []*model.SearchFilter{
					{Property: tc.filterProp1, Values: []*string{&tc.val1}},
					{Property: "cluster", Values: []*string{&tc.cluster}},
					{Property: tc.filterProp2, Values: []*string{&tc.val2}},
				},
				Limit: &limit,
			}
			ud := rbac.UserData{
				CsResources:     []rbac.Resource{},
				ManagedClusters: map[string]struct{}{"test": {}},
			}
			propTypesMock := map[string]string{"cluster": "string", "kind": "string", "container": "array",
				"label": "object", "namespace": "string"}

			resolver, mockPool := newMockSearchResolver(t, searchInput, nil, ud, propTypesMock)
			mockRows := newMockRowsWithoutRBAC("./mocks/mock.json", searchInput, "string", limit)

			mockPool.EXPECT().Query(gomock.Any(), gomock.Eq(tc.expectedQuery), gomock.Eq([]interface{}{})).Return(mockRows, nil)

			result, err := resolver.Items()
			assert.Nil(t, err)
			assert.Len(t, result, len(mockRows.mockData))

			for i, item := range result {
				mockRow := mockRows.mockData[i]
				expectedRow := formatDataMap(mockRow["data"].(map[string]interface{}))
				expectedRow["_uid"] = mockRow["uid"]
				expectedRow["cluster"] = mockRow["cluster"]

				assert.Equal(t, expectedRow, item)
			}
		})
	}
}

func Test_decodePropertyTypes(t *testing.T) {
	_, err := decodePropertyTypes([]string{"!master"}, "array")
	assert.Nil(t, err, "expected no error")
}

func TestMatchesManagedHubFilter(t *testing.T) {
	type test struct {
		name        string
		val1        string
		filterProp1 string
		expectedRes bool
	}
	config.Cfg.HubName = "test-hub-a"

	tests := []test{
		{
			name:        "Match hub name",
			val1:        "test-hub-a",
			filterProp1: "managedHub",
			expectedRes: true,
		},
		{
			name:        "Not Match hub name",
			val1:        "test-hub-b",
			filterProp1: "managedHub",
			expectedRes: false,
		},
		{
			name:        "Not hub name operator !",
			val1:        "!test-hub-a",
			filterProp1: "managedHub",
			expectedRes: false,
		},
		{
			name:        "Not Equal to hub name operator !=",
			val1:        "!=test-hub-a",
			filterProp1: "managedHub",
			expectedRes: false,
		},
		{
			name:        "Partial Match hub name operator",
			val1:        "*test-*",
			filterProp1: "managedHub",
			expectedRes: true,
		},
		{
			name:        "Partial Match Not hub name operator !",
			val1:        "!*test-*",
			filterProp1: "managedHub",
			expectedRes: false,
		},
		{
			name:        "1 Partial Match Not Equal to hub name operator !=",
			val1:        "!=*-hub-*",
			filterProp1: "managedHub",
			expectedRes: false,
		},
		{
			name:        "Partial Match hub name operator - unmatched pattern at start",
			val1:        "=hub-*",
			filterProp1: "managedHub",
			expectedRes: false,
		},
		{
			name:        "Partial Match hub name operator - unmatched pattern at end",
			val1:        "=*hub",
			filterProp1: "managedHub",
			expectedRes: false,
		},
		{
			name:        "Partial Match hub name operator =",
			val1:        "=*hub-a",
			filterProp1: "managedHub",
			expectedRes: true,
		},
		{
			name:        "Error in pattern",
			val1:        "=hub*(",
			filterProp1: "managedHub",
			expectedRes: false,
		},
	}

	limit := 10
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			searchInput := &model.SearchInput{
				Filters: []*model.SearchFilter{
					{Property: tc.filterProp1, Values: []*string{&tc.val1}},
				},
				Limit: &limit,
			}
			ud := rbac.UserData{
				CsResources:     []rbac.Resource{},
				ManagedClusters: map[string]struct{}{"test": {}},
			}
			propTypesMock := map[string]string{"cluster": "string", "kind": "string", "container": "array",
				"label": "object", "namespace": "string", "managedHub": "string"}

			resolver, _ := newMockSearchResolver(t, searchInput, nil, ud, propTypesMock)
			result := resolver.matchesManagedHubFilter()
			assert.Equal(t, tc.expectedRes, result)
		})
	}
}

func Test_buildRbacWhereClause_clusterAdmin(t *testing.T) {
	mock_userData := rbac.UserData{IsClusterAdmin: true}

	result := buildRbacWhereClause(context.Background(), mock_userData, v1.UserInfo{})

	assert.Equal(t, 0, len(result.Expressions()))
}

func Test_buildRbacWhereClause_fineGrainedRBAC_noNamespaces(t *testing.T) {
	config.Cfg.Features.FineGrainedRbac = true
	mock_userData := rbac.UserData{IsClusterAdmin: false, NsResources: map[string][]rbac.Resource{"ns-1": []rbac.Resource{{Kind: "Pod"}}}}

	result := buildRbacWhereClause(context.Background(), mock_userData, v1.UserInfo{})

	sql, _, err := goqu.From("t").Where(result).ToSQL()

	expectedSql := `SELECT * FROM "t" WHERE ("data"?'_hubClusterResource' AND (data->'namespace'?|'{"ns-1"}' AND (NOT("data"?'apigroup') AND data->'kind_plural'?'Pod')))`
	assert.Nil(t, err)
	assert.Equal(t, expectedSql, sql)
}

func Test_buildRbacWhereClause_fineGrainedRBAC(t *testing.T) {
	config.Cfg.Features.FineGrainedRbac = true
	mock_userData := rbac.UserData{IsClusterAdmin: false, FGRbacNamespaces: map[string][]string{"cluster-a": []string{"namespace-a1"}}}

	result := buildRbacWhereClause(context.Background(), mock_userData, v1.UserInfo{})
	sql, _, err := goqu.From("t").Where(result).ToSQL()

	assert.Nil(t, err)
	assert.Contains(t, sql, `(("cluster" = 'cluster-a') AND data->'namespace'?|'{"namespace-a1"}')`)

	// NOTE: We can't validate the entire expresionString because the order ot the expressions isn't
	//      guaranteed. Leaving this here as it would improve this test if we could validate it consistently.
	//
	// expressionString := buildExpressionStringFrom(result)
	// expectedExpression := `(((data->'apigroup'?'kubevirt.io' AND data->'kind'?|'{"VirtualMachine","VirtualMachineInstance","VirtualMachineInstanceMigration","VirtualMachineInstancePreset","VirtualMachineInstanceReplicaset"}') OR (data->'apigroup'?'clone.kubevirt.io' AND data->'kind'?|'{"VirtualMachineClone"}') OR (data->'apigroup'?'export.kubevirt.io' AND data->'kind'?|'{"VirtualMachineExport"}') OR (data->'apigroup'?'instancetype.kubevirt.io' AND data->'kind'?|'{"VirtualMachineClusterInstancetype","VirtualMachineClusterPreference","VirtualMachineInstancetype","VirtualMachinePreference"}') OR (data->'apigroup'?'migrations.kubevirt.io' AND data->'kind'?|'{"MigrationPolicy"}') OR (data->'apigroup'?'pool.kubevirt.io' AND data->'kind'?|'{"VirtualMachinePool"}') OR (data->'apigroup'?'snapshot.kubevirt.io' AND data->'kind'?|'{"VirtualMachineRestore","VirtualMachineSnapshot","VirtualMachineSnapshotContent"}')) AND (("cluster" = 'cluster-a') AND data->'namespace'?|'{"namespace-a1"}'))`
	// assert.Equal(t, expectedExpression, expressionString)
}

// Test_ExtractOrderByProperty_WithDirection tests that the property name is correctly
// extracted from an orderBy string that includes a direction (asc/desc).
// Scenario: orderBy = "name desc"
// Expected: Returns "name"
func Test_ExtractOrderByProperty_WithDirection(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := "name desc"

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Execute function
	property := resolver.extractOrderByProperty()
	assert.Equal(t, "name", property, "Should extract property name before space")
}

// Test_ExtractOrderByProperty_WithoutDirection tests property extraction when
// only the property name is provided without a direction.
// Scenario: orderBy = "namespace"
// Expected: Returns "namespace"
func Test_ExtractOrderByProperty_WithoutDirection(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := "namespace"

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Execute function
	property := resolver.extractOrderByProperty()
	assert.Equal(t, "namespace", property, "Should return entire string when no space")
}

// Test_ExtractOrderByProperty_EmptyString tests behavior with empty orderBy string.
// Scenario: orderBy = ""
// Expected: Returns empty string
func Test_ExtractOrderByProperty_EmptyString(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := ""

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Execute function
	property := resolver.extractOrderByProperty()
	assert.Equal(t, "", property, "Should return empty string for empty orderBy")
}

// Test_ExtractOrderByProperty_NilOrderBy tests behavior when orderBy is nil.
// Scenario: orderBy = nil
// Expected: Returns empty string
func Test_ExtractOrderByProperty_NilOrderBy(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: nil,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Execute function
	property := resolver.extractOrderByProperty()
	assert.Equal(t, "", property, "Should return empty string for nil orderBy")
}

// Test_ExtractOrderByProperty_MultipleSpaces tests parsing with multiple spaces
// between property and direction.
// Scenario: orderBy = "name  desc" (double space)
// Expected: Returns "name" (extracts property before first space)
func Test_ExtractOrderByProperty_MultipleSpaces(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := "name  desc" // Double space

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Execute function
	property := resolver.extractOrderByProperty()
	assert.Equal(t, "name", property, "Should extract property before first space")
}

// Test_ExtractOrderByProperty_SpecialCharacters tests property extraction with
// special characters commonly found in JSON property names.
// Scenario: orderBy = "app-version asc"
// Expected: Returns "app-version"
func Test_ExtractOrderByProperty_SpecialCharacters(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := "app-version asc"

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Execute function
	property := resolver.extractOrderByProperty()
	assert.Equal(t, "app-version", property, "Should extract property with hyphens")
}

// Test_ExtractOrderByProperty_LeadingSpace tests behavior when orderBy starts with a space.
// The function should trim leading/trailing whitespace and extract the property correctly.
// Scenario: orderBy = " name asc" (leading space)
// Expected: Returns "name" (whitespace trimmed before parsing)
func Test_ExtractOrderByProperty_LeadingSpace(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := " name asc" // Leading space

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Execute function
	property := resolver.extractOrderByProperty()
	assert.Equal(t, "name", property, "Should trim leading space and extract property name")
}

// Test_ExtractOrderByProperty_TrailingSpace tests behavior when orderBy ends with a space.
// The function should trim leading/trailing whitespace.
// Scenario: orderBy = "name " (trailing space)
// Expected: Returns "name" (whitespace trimmed)
func Test_ExtractOrderByProperty_TrailingSpace(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := "name " // Trailing space

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Execute function
	property := resolver.extractOrderByProperty()
	assert.Equal(t, "name", property, "Should trim trailing space and return property name")
}

// Test_ExtractOrderByProperty_OnlyWhitespace tests behavior when orderBy contains only whitespace.
// Scenario: orderBy = "   " (only spaces)
// Expected: Returns empty string (nothing left after trimming)
func Test_ExtractOrderByProperty_OnlyWhitespace(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := "   " // Only whitespace

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Execute function
	property := resolver.extractOrderByProperty()
	assert.Equal(t, "", property, "Should return empty string when orderBy is only whitespace")
}

// Test_Items_OrderByOnlyWhitespace tests that when orderBy contains only whitespace,
// an error is returned because it's an invalid format.
// This tests the else branch in SELECT building and validation in applyOrderBy.
// Scenario: orderBy = "   " (only whitespace)
// Expected: Returns error "invalid orderBy format"
func Test_Items_OrderByOnlyWhitespace(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := "   " // Only whitespace

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Execute function - should return error from applyOrderBy validation
	// Note: line 204 is executed (SELECT without order column) before validation fails
	items, err := resolver.Items()
	assert.NotNil(t, err, "Should return error for whitespace-only orderBy")
	assert.Contains(t, err.Error(), "invalid orderBy format", "Error should mention invalid format")
	assert.Nil(t, items, "Items should be nil when error occurs")
}

// Test_Items_InvalidOrderByDirection tests that when orderBy has an invalid direction,
// an error is returned through the full query building path.
// This tests the error handling path in buildSearchQuery when applyOrderBy returns an error.
// Scenario: orderBy = "name invaliddir" (invalid direction)
// Expected: Returns error "invalid orderBy direction"
func Test_Items_InvalidOrderByDirection(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := "name invaliddir" // Invalid direction

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Execute function - should return error from applyOrderBy validation
	items, err := resolver.Items()
	assert.NotNil(t, err, "Should return error for invalid orderBy direction")
	assert.Contains(t, err.Error(), "invalid orderBy direction", "Error should mention invalid direction")
	assert.Nil(t, items, "Items should be nil when error occurs")
}

// Test_ApplyOrderBy_AscendingOrder validates that the ORDER BY clause is correctly
// applied with ascending sort direction.
// Scenario: orderBy = "name asc"
// Expected: SQL contains "ORDER BY ... ASC"
func Test_ApplyOrderBy_AscendingOrder(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := "name asc"

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Create a base query dataset
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable).Select("uid", "cluster", "data")

	// Apply orderBy
	result, err := resolver.applyOrderBy(ds)
	assert.Nil(t, err, "Should not return error for valid orderBy")

	// Get the SQL to verify ORDER BY clause
	sql, _, err := result.ToSQL()
	assert.Nil(t, err)
	assert.Contains(t, sql, "ORDER BY", "SQL should contain ORDER BY clause")
	assert.Contains(t, sql, "ASC", "SQL should contain ASC direction")
}

// Test_ApplyOrderBy_DescendingOrder validates that the ORDER BY clause is correctly
// applied with descending sort direction.
// Scenario: orderBy = "created desc"
// Expected: SQL contains "ORDER BY ... DESC"
func Test_ApplyOrderBy_DescendingOrder(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := "created desc"

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Create a base query dataset
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable).Select("uid", "cluster", "data")

	// Apply orderBy
	result, err := resolver.applyOrderBy(ds)
	assert.Nil(t, err, "Should not return error for valid orderBy")

	// Get the SQL to verify ORDER BY clause
	sql, _, err := result.ToSQL()
	assert.Nil(t, err)
	assert.Contains(t, sql, "ORDER BY", "SQL should contain ORDER BY clause")
	assert.Contains(t, sql, "DESC", "SQL should contain DESC direction")
}

// Test_ApplyOrderBy_DefaultDirection validates that when no direction is specified,
// the system defaults to ascending order.
// Scenario: orderBy = "namespace" (no direction specified)
// Expected: SQL contains "ORDER BY ... ASC" (defaults to ascending)
func Test_ApplyOrderBy_DefaultDirection(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := "namespace" // No direction specified, should default to asc

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Create a base query dataset
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable).Select("uid", "cluster", "data")

	// Apply orderBy
	result, err := resolver.applyOrderBy(ds)
	assert.Nil(t, err, "Should not return error for valid orderBy")

	// Get the SQL to verify ORDER BY clause defaults to ASC
	sql, _, err := result.ToSQL()
	assert.Nil(t, err)
	assert.Contains(t, sql, "ORDER BY", "SQL should contain ORDER BY clause")
	assert.Contains(t, sql, "ASC", "SQL should default to ASC when no direction specified")
}

// Test_ApplyOrderBy_EmptyString validates that an empty orderBy string
// does not add any ORDER BY clause to the query.
// Scenario: orderBy = ""
// Expected: SQL does NOT contain "ORDER BY"
func Test_ApplyOrderBy_EmptyString(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := ""

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Create a base query dataset
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable).Select("uid", "cluster", "data")

	// Apply orderBy
	result, err := resolver.applyOrderBy(ds)
	assert.Nil(t, err, "Should not return error for valid orderBy")

	// Get the SQL to verify no ORDER BY clause is added
	sql, _, err := result.ToSQL()
	assert.Nil(t, err)
	assert.NotContains(t, sql, "ORDER BY", "SQL should not contain ORDER BY clause when orderBy is empty")
}

// Test_ApplyOrderBy_OnlyWhitespace validates that orderBy with only whitespace returns an error.
// This tests the validation when strings.Fields returns empty slice.
// Scenario: orderBy = "   " (only spaces)
// Expected: Returns error "invalid orderBy format"
func Test_ApplyOrderBy_OnlyWhitespace(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := "   " // Only whitespace

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Create a base query dataset
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable).Select("uid", "cluster", "data")

	// Apply orderBy - should return error
	result, err := resolver.applyOrderBy(ds)
	assert.NotNil(t, err, "Should return error for whitespace-only orderBy")
	assert.Contains(t, err.Error(), "invalid orderBy format", "Error should mention invalid format")
	assert.Nil(t, result, "Result should be nil when error occurs")
}

// Test_ApplyOrderBy_NilOrderBy validates that a nil orderBy parameter
// does not add any ORDER BY clause to the query.
// Scenario: orderBy = nil
// Expected: SQL does NOT contain "ORDER BY"
func Test_ApplyOrderBy_NilOrderBy(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: nil,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Create a base query dataset
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable).Select("uid", "cluster", "data")

	// Apply orderBy
	result, err := resolver.applyOrderBy(ds)
	assert.Nil(t, err, "Should not return error for valid orderBy")

	// Get the SQL to verify no ORDER BY clause is added
	sql, _, err := result.ToSQL()
	assert.Nil(t, err)
	assert.NotContains(t, sql, "ORDER BY", "SQL should not contain ORDER BY clause when orderBy is nil")
}

// Test_ApplyOrderBy_InvalidDirection validates that an invalid direction returns an error.
// Scenario: orderBy = "name descrr" (typo in direction)
// Expected: Returns error indicating invalid direction
func Test_ApplyOrderBy_InvalidDirection(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := "name descrr" // Invalid direction (typo)

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Create a base query dataset
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable).Select("uid", "cluster", "data")

	// Apply orderBy
	_, err := resolver.applyOrderBy(ds)
	assert.NotNil(t, err, "Should return error for invalid direction")
	assert.Contains(t, err.Error(), "invalid orderBy direction", "Error should mention invalid direction")
	assert.Contains(t, err.Error(), "descrr", "Error should mention the invalid value")
}

// Test_ApplyOrderBy_ExtraValues validates that extra values beyond property and direction return an error.
// Scenario: orderBy = "name desc extra values" (more than 2 parts)
// Expected: Returns error indicating too many parts
func Test_ApplyOrderBy_ExtraValues(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := "name desc extra values" // Extra values

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Create a base query dataset
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable).Select("uid", "cluster", "data")

	// Apply orderBy
	_, err := resolver.applyOrderBy(ds)
	assert.NotNil(t, err, "Should return error for extra values")
	assert.Contains(t, err.Error(), "invalid orderBy format", "Error should mention invalid format")
	assert.Contains(t, err.Error(), "4 parts", "Error should mention the number of parts found")
}

// Test_ApplyOrderBy_CaseInsensitiveDirection validates that direction is case-insensitive.
// Scenario: orderBy = "name DESC" (uppercase), "name AsC" (mixed case)
// Expected: Both work correctly
func Test_ApplyOrderBy_CaseInsensitiveDirection(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"

	// Test uppercase DESC
	orderByUpper := "name DESC"
	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderByUpper,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable).Select("uid", "cluster", "data")
	result, err := resolver.applyOrderBy(ds)
	assert.Nil(t, err, "Should not return error for valid orderBy")

	sql, _, err := result.ToSQL()
	assert.Nil(t, err)
	assert.Contains(t, sql, "DESC", "Should handle uppercase DESC")

	// Test mixed case ASC
	orderByMixed := "name AsC"
	searchInput2 := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderByMixed,
	}
	resolver2, _ := newMockSearchResolver(t, searchInput2, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	ds2 := goqu.From(schemaTable).Select("uid", "cluster", "data")
	result2, err := resolver2.applyOrderBy(ds2)
	assert.Nil(t, err, "Should not return error for valid orderBy")

	sql2, _, err2 := result2.ToSQL()
	assert.Nil(t, err2)
	assert.Contains(t, sql2, "ASC", "Should handle mixed case AsC")
}

// Test_BuildSearchQuery_WithOffsetAndOrderBy validates that all pagination parameters
// (offset, limit, orderBy) are correctly integrated into the SQL query.
// This is an integration test ensuring all features work together.
// Scenario: offset 20, limit 10, orderBy "name desc"
// Expected: SQL contains "OFFSET 20", "LIMIT 10", and "ORDER BY ... DESC"
func Test_BuildSearchQuery_WithOffsetAndOrderBy(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	offset := 20
	limit := 10
	orderBy := "name desc"

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		Offset:  &offset,
		Limit:   &limit,
		OrderBy: &orderBy,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Build the query
	err := resolver.buildSearchQuery(resolver.context, false, false)
	assert.Nil(t, err)

	// Verify the query contains OFFSET, LIMIT, and ORDER BY
	assert.Contains(t, resolver.query, "OFFSET 20", "Query should contain OFFSET clause")
	assert.Contains(t, resolver.query, "LIMIT 10", "Query should contain LIMIT clause")
	assert.Contains(t, resolver.query, "ORDER BY", "Query should contain ORDER BY clause")
	assert.Contains(t, resolver.query, "DESC", "Query should contain DESC direction")
}

// Test_BuildSearchQuery_WithOnlyOffset validates that offset and limit work correctly
// without requiring an orderBy parameter.
// Scenario: offset 50, limit 25, no orderBy
// Expected: SQL contains "OFFSET 50" and "LIMIT 25" but NOT "ORDER BY"
func Test_BuildSearchQuery_WithOnlyOffset(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	offset := 50
	limit := 25

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		Offset:  &offset,
		Limit:   &limit,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Build the query
	err := resolver.buildSearchQuery(resolver.context, false, false)
	assert.Nil(t, err)

	// Verify the query contains OFFSET and LIMIT but not ORDER BY
	assert.Contains(t, resolver.query, "OFFSET 50", "Query should contain OFFSET clause")
	assert.Contains(t, resolver.query, "LIMIT 25", "Query should contain LIMIT clause")
	assert.NotContains(t, resolver.query, "ORDER BY", "Query should not contain ORDER BY when not specified")
}

// Test_BuildSearchQuery_NegativeOffset validates that negative offset values return an error.
// Offset must be non-negative (>= 0).
// Scenario: offset = -50
// Expected: Returns error indicating invalid offset
func Test_BuildSearchQuery_NegativeOffset(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	offset := -50
	limit := 10

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		Offset:  &offset,
		Limit:   &limit,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Build the query
	err := resolver.buildSearchQuery(resolver.context, false, false)
	assert.NotNil(t, err, "Should return error for negative offset")
	assert.Contains(t, err.Error(), "invalid offset", "Error should mention invalid offset")
	assert.Contains(t, err.Error(), "-50", "Error should mention the invalid value")
}

// Test_BuildSearchQuery_ZeroOffset validates that offset=0 is valid but doesn't add OFFSET clause.
// Zero offset means start from the beginning, which is the default behavior.
// Scenario: offset = 0
// Expected: Query builds successfully without OFFSET clause (since it's redundant)
func Test_BuildSearchQuery_ZeroOffset(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	offset := 0
	limit := 10

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		Offset:  &offset,
		Limit:   &limit,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Build the query
	err := resolver.buildSearchQuery(resolver.context, false, false)
	assert.Nil(t, err, "Should not return error for offset=0")

	// Verify the query does NOT contain OFFSET (since 0 is redundant)
	assert.NotContains(t, resolver.query, "OFFSET", "Query should not contain OFFSET clause for offset=0")
	assert.Contains(t, resolver.query, "LIMIT 10", "Query should contain LIMIT clause")
}

// Test_BuildSearchQuery_CountIgnoresOffsetAndOrderBy validates that count queries
// do not include pagination parameters (OFFSET, LIMIT, ORDER BY).
// This ensures count queries return the total number of matching items, not page-specific counts.
// Scenario: count query with offset 20, limit 10, orderBy "name desc"
// Expected: SQL contains "COUNT" but NOT "OFFSET", "LIMIT", or "ORDER BY"
func Test_BuildSearchQuery_CountIgnoresOffsetAndOrderBy(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	offset := 20
	limit := 10
	orderBy := "name desc"

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		Offset:  &offset,
		Limit:   &limit,
		OrderBy: &orderBy,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Build the query with count=true
	err := resolver.buildSearchQuery(resolver.context, true, false)
	assert.Nil(t, err)

	// Verify the query does NOT contain OFFSET, LIMIT, or ORDER BY for count queries
	assert.NotContains(t, resolver.query, "OFFSET", "Count query should not contain OFFSET")
	assert.NotContains(t, resolver.query, "LIMIT", "Count query should not contain LIMIT")
	assert.NotContains(t, resolver.query, "ORDER BY", "Count query should not contain ORDER BY")
	assert.Contains(t, resolver.query, "COUNT", "Count query should contain COUNT")
}

// Test_ResolveItems_WithOrderBy validates that resolveItems correctly handles
// the extra column in SELECT when orderBy is specified.
// This tests the fix for PostgreSQL's "SELECT DISTINCT ... ORDER BY" constraint,
// which requires ORDER BY expressions to appear in the SELECT list.
// When orderBy is used, we add data->>'property' to SELECT, creating 4 columns instead of 3.
// The resolveItems function must scan all 4 columns to avoid the error:
// "number of field descriptions must equal number of destinations"
// Scenario: Query with orderBy="name asc" and items requested
// Expected: Items are returned successfully, scanning 4 columns (uid, cluster, data, order field)
func Test_ResolveItems_WithOrderBy(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := "name asc"
	limit := 10

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
		Limit:   &limit,
	}
	resolver, mockPool := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Build the query first
	err := resolver.buildSearchQuery(resolver.context, false, false)
	assert.Nil(t, err)

	// Verify the query includes the order field in SELECT
	assert.Contains(t, resolver.query, "data->>'name'", "Query should include order field in SELECT")

	// Mock the database response with 4 columns (uid, cluster, data, order_field)
	mockData := []map[string]interface{}{
		{
			"uid":     "local-cluster/pod1",
			"cluster": "local-cluster",
			"data": map[string]interface{}{
				"kind":      "Pod",
				"name":      "pod-alpha",
				"namespace": "default",
			},
			"order_field": "pod-alpha",
		},
		{
			"uid":     "local-cluster/pod2",
			"cluster": "local-cluster",
			"data": map[string]interface{}{
				"kind":      "Pod",
				"name":      "pod-beta",
				"namespace": "default",
			},
			"order_field": "pod-beta",
		},
	}

	mockRows := &MockRows{
		mockData:      mockData,
		index:         0,
		columnHeaders: []string{"uid", "cluster", "data", "order_field"},
	}

	mockPool.EXPECT().Query(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockRows, nil)

	// Execute resolveItems
	items, err := resolver.resolveItems()
	assert.Nil(t, err)
	assert.NotNil(t, items)
	assert.Equal(t, 2, len(items), "Should return 2 items")

	// Verify items contain the expected data
	assert.Equal(t, "Pod", items[0]["kind"])
	assert.Equal(t, "pod-alpha", items[0]["name"])
	assert.Equal(t, "local-cluster", items[0]["cluster"])

	assert.Equal(t, "Pod", items[1]["kind"])
	assert.Equal(t, "pod-beta", items[1]["name"])
	assert.Equal(t, "local-cluster", items[1]["cluster"])
}

// Test_ResolveItems_WithOrderBy_NoItems validates that when orderBy is specified
// but items are NOT requested, the query should NOT include the extra order column.
// This is important because count-only queries or queries that only request related
// resources should not be affected by the SELECT DISTINCT constraint.
// Scenario: Query with orderBy="name asc" but only requesting count (no items)
// Expected: Query does NOT contain the order field in SELECT (3 columns only)
func Test_ResolveItems_WithOrderBy_NoItems(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := "name asc"

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
	}
	resolver, mockPool := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Build the query for count (not items)
	err := resolver.buildSearchQuery(resolver.context, true, false)
	assert.Nil(t, err)

	// Verify the query does NOT include order field in SELECT for count queries
	assert.NotContains(t, resolver.query, "data->>'name'", "Count query should NOT include order field in SELECT")
	assert.Contains(t, resolver.query, "COUNT", "Should be a count query")

	// Mock the count query response
	mockRow := &Row{MockValue: 42}
	mockPool.EXPECT().QueryRow(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockRow)

	// Execute count
	count, err := resolver.Count()
	assert.Nil(t, err)
	assert.Equal(t, 42, count)
}

// Test_Pagination_WithKeywords tests that pagination (offset/limit) works correctly
// when combined with keyword search (text matching across all fields).
// Keywords should not interfere with pagination functionality.
// Scenario: Search with keywords="backup", offset=10, limit=5
// Expected: Query builds successfully with both keyword filters and pagination
func Test_Pagination_WithKeywords(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	keyword1 := "backup"
	offset := 10
	limit := 5

	searchInput := &model.SearchInput{
		Keywords: []*string{&keyword1},
		Filters:  []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		Offset:   &offset,
		Limit:    &limit,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Build the query
	err := resolver.buildSearchQuery(resolver.context, false, false)
	assert.Nil(t, err)

	// Verify query contains both keyword handling and pagination
	assert.Contains(t, resolver.query, "OFFSET 10", "Query should contain OFFSET for pagination")
	assert.Contains(t, resolver.query, "LIMIT 5", "Query should contain LIMIT for pagination")
	// Keywords are handled via jsonb_each_text in the FROM clause
	assert.NotEmpty(t, resolver.query, "Query should be built successfully with keywords")
}

// Test_OrderBy_WithKeywords tests that sorting (orderBy) works correctly when combined
// with keyword search. This is a complex scenario because keywords modify the FROM clause
// by adding jsonb_each_text, which could interfere with ORDER BY.
// Scenario: Search with keywords="cluster" and orderBy="name desc"
// Expected: Query builds with both keyword handling and ORDER BY clause
func Test_OrderBy_WithKeywords(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	keyword1 := "cluster"
	orderBy := "name desc"
	limit := 10

	searchInput := &model.SearchInput{
		Keywords: []*string{&keyword1},
		Filters:  []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy:  &orderBy,
		Limit:    &limit,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Build the query
	err := resolver.buildSearchQuery(resolver.context, false, false)
	assert.Nil(t, err)

	// Verify query contains both keyword handling and ORDER BY
	assert.Contains(t, resolver.query, "ORDER BY", "Query should contain ORDER BY clause")
	assert.Contains(t, resolver.query, "DESC", "Query should contain DESC direction")
	assert.Contains(t, resolver.query, "data->>'name'", "Query should include order field in SELECT")
	assert.NotEmpty(t, resolver.query, "Query should be built successfully")
}

// Test_OrderBy_WithRelatedKinds tests that orderBy works when relatedKinds filter is specified.
// The relatedKinds parameter filters which related resources to include in the 'related' field,
// but should not affect the main items query or ordering.
// Scenario: Query with orderBy="name asc" and relatedKinds=["ReplicaSet"]
// Expected: Query builds successfully, orderBy applies to main items
func Test_OrderBy_WithRelatedKinds(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Deployment"
	orderBy := "name asc"
	relatedKind1 := "ReplicaSet"
	relatedKind2 := "Pod"
	limit := 10

	searchInput := &model.SearchInput{
		Filters:      []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy:      &orderBy,
		RelatedKinds: []*string{&relatedKind1, &relatedKind2},
		Limit:        &limit,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Build the query
	err := resolver.buildSearchQuery(resolver.context, false, false)
	assert.Nil(t, err)

	// Verify query contains ORDER BY
	assert.Contains(t, resolver.query, "ORDER BY", "Query should contain ORDER BY clause")
	assert.Contains(t, resolver.query, "ASC", "Query should contain ASC direction")
	assert.Contains(t, resolver.query, "data->>'name'", "Query should include order field in SELECT")

	// RelatedKinds are stored but don't affect the main query
	assert.Equal(t, 2, len(resolver.input.RelatedKinds), "RelatedKinds should be stored")
}

// Test_FullPagination_AllFeatures tests the most complex scenario with all pagination
// features enabled simultaneously: keywords, filters, offset, limit, orderBy, and relatedKinds.
// This is a comprehensive integration test to ensure all features work together without conflicts.
// Scenario: All SearchInput options specified together
// Expected: Query builds successfully with all features integrated
func Test_FullPagination_AllFeatures(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string", "namespace": "string"}
	kind := "Pod"
	namespace := "default"
	keyword := "web"
	orderBy := "created desc"
	relatedKind := "Service"
	offset := 5
	limit := 10

	searchInput := &model.SearchInput{
		Keywords: []*string{&keyword},
		Filters: []*model.SearchFilter{
			{Property: "kind", Values: []*string{&kind}},
			{Property: "namespace", Values: []*string{&namespace}},
		},
		Offset:       &offset,
		Limit:        &limit,
		OrderBy:      &orderBy,
		RelatedKinds: []*string{&relatedKind},
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Build the query
	err := resolver.buildSearchQuery(resolver.context, false, false)
	assert.Nil(t, err)

	// Verify all pagination features are present in the query
	assert.Contains(t, resolver.query, "OFFSET 5", "Query should contain OFFSET")
	assert.Contains(t, resolver.query, "LIMIT 10", "Query should contain LIMIT")
	assert.Contains(t, resolver.query, "ORDER BY", "Query should contain ORDER BY")
	assert.Contains(t, resolver.query, "DESC", "Query should contain DESC direction")
	assert.Contains(t, resolver.query, "data->>'created'", "Query should include order field in SELECT")
	assert.NotEmpty(t, resolver.query, "Query should be built successfully with all features")
}

// Test_Pagination_LargeOffset tests pagination behavior with a large offset value.
// Large offsets (>10,000) can have performance implications in PostgreSQL,
// but should still work correctly.
// Scenario: Query with offset=50000, limit=100
// Expected: Query builds successfully, contains large offset value
func Test_Pagination_LargeOffset(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "ConfigMap"
	offset := 50000
	limit := 100

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		Offset:  &offset,
		Limit:   &limit,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Build the query
	err := resolver.buildSearchQuery(resolver.context, false, false)
	assert.Nil(t, err)

	// Verify large offset is handled
	assert.Contains(t, resolver.query, "OFFSET 50000", "Query should contain large OFFSET value")
	assert.Contains(t, resolver.query, "LIMIT 100", "Query should contain LIMIT")
}

// Test_OrderBy_InvalidProperty tests behavior when orderBy specifies a property
// that may not exist in all resources. The query should still build successfully,
// though results may vary based on whether resources have that property.
// Scenario: orderBy with an uncommon property name
// Expected: Query builds successfully with the specified property
func Test_OrderBy_InvalidProperty(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := "nonexistent_field asc"
	limit := 10

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
		Limit:   &limit,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Build the query
	err := resolver.buildSearchQuery(resolver.context, false, false)
	assert.Nil(t, err)

	// Verify query builds even with non-standard property
	assert.Contains(t, resolver.query, "ORDER BY", "Query should contain ORDER BY clause")
	assert.Contains(t, resolver.query, "data->>'nonexistent_field'", "Query should include specified property")
}

// Test_OrderBy_CaseSensitivity tests that orderBy handles property names correctly.
// PostgreSQL's ->> operator is case-sensitive for JSON keys, so "Name" and "name"
// are different properties.
// Scenario: orderBy with capitalized property name
// Expected: Query uses the exact property name as specified
func Test_OrderBy_CaseSensitivity(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := "Name asc" // Capitalized property name
	limit := 10

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
		Limit:   &limit,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Build the query
	err := resolver.buildSearchQuery(resolver.context, false, false)
	assert.Nil(t, err)

	// Verify exact property name is used (case-sensitive)
	assert.Contains(t, resolver.query, "data->>'Name'", "Query should use exact property name")
	assert.NotContains(t, resolver.query, "data->>'name'", "Query should not lowercase the property")
}

// Test_OrderBy_SpecialCharacters tests that orderBy handles property names with
// special characters (hyphens, dots, underscores). These are valid in JSON keys.
// Scenario: orderBy with property containing hyphens
// Expected: Query builds successfully with the property name
func Test_OrderBy_SpecialCharacters(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := "app-version desc"
	limit := 10

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
		Limit:   &limit,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Build the query
	err := resolver.buildSearchQuery(resolver.context, false, false)
	assert.Nil(t, err)

	// Verify property with special characters is handled
	assert.Contains(t, resolver.query, "data->>'app-version'", "Query should include property with hyphens")
	assert.Contains(t, resolver.query, "ORDER BY", "Query should contain ORDER BY clause")
}

// Test_OrderBy_ClusterColumn tests ordering by 'cluster', which is a table column not in jsonb.
// The code should reference the 'cluster' column directly, not try to extract it from 'data'.
// Scenario: orderBy = "cluster asc"
// Expected: Query uses "ORDER BY cluster ASC", not "data->>'cluster'"
func Test_OrderBy_ClusterColumn(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := "cluster asc"
	limit := 10

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
		Limit:   &limit,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Build the query
	err := resolver.buildSearchQuery(resolver.context, false, false)
	assert.Nil(t, err)

	// Verify 'cluster' is used as direct column reference, not jsonb extraction
	assert.Contains(t, resolver.query, "ORDER BY", "Query should contain ORDER BY clause")
	assert.Contains(t, resolver.query, "\"cluster\"", "Query should reference cluster column directly")
	assert.NotContains(t, resolver.query, "data->>'cluster'", "Query should NOT extract cluster from jsonb")
}

// Test_OrderBy_UidColumn tests ordering by 'uid', which is a table column not in jsonb.
// The code should reference the 'uid' column directly, not try to extract it from 'data'.
// Scenario: orderBy = "uid desc"
// Expected: Query uses "ORDER BY uid DESC", not "data->>'uid'"
func Test_OrderBy_UidColumn(t *testing.T) {
	propTypesMock := map[string]string{"kind": "string"}
	val1 := "Pod"
	orderBy := "uid desc"
	limit := 10

	searchInput := &model.SearchInput{
		Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}},
		OrderBy: &orderBy,
		Limit:   &limit,
	}
	resolver, _ := newMockSearchResolver(t, searchInput, nil, rbac.UserData{CsResources: []rbac.Resource{}}, propTypesMock)

	// Build the query
	err := resolver.buildSearchQuery(resolver.context, false, false)
	assert.Nil(t, err)

	// Verify 'uid' is used as direct column reference, not jsonb extraction
	assert.Contains(t, resolver.query, "ORDER BY", "Query should contain ORDER BY clause")
	assert.Contains(t, resolver.query, "\"uid\"", "Query should reference uid column directly")
	assert.NotContains(t, resolver.query, "data->>'uid'", "Query should NOT extract uid from jsonb")
}
