// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/rbac"
)

func Test_Messages_Query(t *testing.T) {
	// Create a SearchSchemaResolver instance with a mock connection pool.
	resolver, _ := newMockMessage(t, &rbac.UserData{})

	sql := `SELECT DISTINCT "mcInfo".data->>'name' AS "srchAddonDisabledCluster" FROM "search"."resources" AS "mcInfo" LEFT OUTER JOIN "search"."resources" AS "srchAddon" ON (("mcInfo".data->>'name' = "srchAddon".data->>'namespace') AND ("srchAddon".data->>'kind' = 'ManagedClusterAddOn') AND ("srchAddon".data->>'name' = 'search-collector')) WHERE (("mcInfo".data->>'kind' = 'ManagedClusterInfo') AND ("srchAddon".uid IS NULL) AND ("mcInfo".data->>'name' != 'local-cluster'))`
	// Execute function
	resolver.buildSearchAddonDisabledQuery(context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456"))

	// Verify response
	if resolver.query != sql {
		t.Errorf("Expected sql query: %s but got %s", sql, resolver.query)
	}
}

func Test_Message_Results_NoAccess(t *testing.T) {
	csRes, nsRes, _ := newUserData()
	mc := map[string]struct{}{}
	ud := rbac.UserData{CsResources: csRes, NsResources: nsRes, ManagedClusters: mc}

	// Create a SearchSchemaResolver instance with a mock connection pool.
	resolver, mockPool := newMockMessage(t, &ud)

	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("../resolver/mocks/mock.json", nil, "srchAddonDisabledCluster", 0)
	// Query before rbac
	// SELECT COUNT(DISTINCT("mcInfo".data->>'name')) FROM "search"."resources" AS "mcInfo" LEFT OUTER JOIN "search"."resources" AS "srchAddon" ON (("mcInfo".data->>'name' = "srchAddon".data->>'namespace') AND ("srchAddon".data->>'kind' = 'ManagedClusterAddOn') AND ("srchAddon".data->>'name' = 'search-collector')) WHERE (("mcInfo".data->>'kind' = 'ManagedClusterInfo') AND ("srchAddon".uid IS NULL) AND ("mcInfo".data->>'name' != 'local-cluster'))`),

	// Mock the database query
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "mcInfo".data->>'name' AS "srchAddonDisabledCluster" FROM "search"."resources" AS "mcInfo" LEFT OUTER JOIN "search"."resources" AS "srchAddon" ON (("mcInfo".data->>'name' = "srchAddon".data->>'namespace') AND ("srchAddon".data->>'kind' = 'ManagedClusterAddOn') AND ("srchAddon".data->>'name' = 'search-collector')) WHERE (("mcInfo".data->>'kind' = 'ManagedClusterInfo') AND ("srchAddon".uid IS NULL) AND ("mcInfo".data->>'name' != 'local-cluster'))`),
	).Return(mockRows, nil)

	fmt.Println("mockRows.data: ", mockRows.mockData)
	ctx := context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456")
	resolver.buildSearchAddonDisabledQuery(ctx)
	//Execute the function
	res, err := resolver.messageResults(ctx)

	messages := make([]*model.Message, 0)

	if !reflect.DeepEqual(messages, res) {
		t.Errorf("Message results doesn't match. Expected: %#v, Got: %#v", messages, res)
	}
	if err != nil {
		t.Errorf("Incorrect results. expected error to be [%v] got [%v]", nil, err)
	}
}

func Test_Message_Results_WithAccess(t *testing.T) {
	csRes, nsRes, mc := newUserData()
	ud := rbac.UserData{CsResources: csRes, NsResources: nsRes, ManagedClusters: mc}

	// Create a SearchSchemaResolver instance with a mock connection pool.
	resolver, mockPool := newMockMessage(t, &ud)

	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("../resolver/mocks/mock.json", nil, "srchAddonDisabledCluster", 0)
	// Query before rbac
	// SELECT COUNT(DISTINCT("mcInfo".data->>'name')) FROM "search"."resources" AS "mcInfo" LEFT OUTER JOIN "search"."resources" AS "srchAddon" ON (("mcInfo".data->>'name' = "srchAddon".data->>'namespace') AND ("srchAddon".data->>'kind' = 'ManagedClusterAddOn') AND ("srchAddon".data->>'name' = 'search-collector')) WHERE (("mcInfo".data->>'kind' = 'ManagedClusterInfo') AND ("srchAddon".uid IS NULL) AND ("mcInfo".data->>'name' != 'local-cluster'))`),

	// Mock the database query
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "mcInfo".data->>'name' AS "srchAddonDisabledCluster" FROM "search"."resources" AS "mcInfo" LEFT OUTER JOIN "search"."resources" AS "srchAddon" ON (("mcInfo".data->>'name' = "srchAddon".data->>'namespace') AND ("srchAddon".data->>'kind' = 'ManagedClusterAddOn') AND ("srchAddon".data->>'name' = 'search-collector')) WHERE (("mcInfo".data->>'kind' = 'ManagedClusterInfo') AND ("srchAddon".uid IS NULL) AND ("mcInfo".data->>'name' != 'local-cluster'))`),
	).Return(mockRows, nil)
	fmt.Println("mockRows.data: ", mockRows.mockData)
	ctx := context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456")
	resolver.buildSearchAddonDisabledQuery(ctx)
	//Execute the function
	res, err := resolver.messageResults(ctx)

	messages := make([]*model.Message, 0)
	kind := "information"
	desc := "Search is disabled on some of your managed clusters."
	message := model.Message{ID: "S20",
		Kind:        &kind,
		Description: &desc}
	messages = append(messages, &message)

	if !reflect.DeepEqual(messages, res) {
		t.Errorf("Message results doesn't match. Expected: %#v, Got: %#v", messages, res)
	}
	if err != nil {
		t.Errorf("Incorrect results. expected error to be [%v] got [%v]", nil, err)
	}
}

func Test_Message_Results_ErrRunningQuery(t *testing.T) {
	csRes, nsRes, mc := newUserData()
	ud := rbac.UserData{CsResources: csRes, NsResources: nsRes, ManagedClusters: mc}
	rbac.CacheInst.SetDisabledClusters(map[string]struct{}{}, fmt.Errorf("error"))
	// Create a SearchSchemaResolver instance with a mock connection pool.
	resolver, mockPool := newMockMessage(t, &ud)

	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("../resolver/mocks/mock.json", nil, "srchAddonDisabledCluster", 0)
	// Query before rbac
	// SELECT COUNT(DISTINCT("mcInfo".data->>'name')) FROM "search"."resources" AS "mcInfo" LEFT OUTER JOIN "search"."resources" AS "srchAddon" ON (("mcInfo".data->>'name' = "srchAddon".data->>'namespace') AND ("srchAddon".data->>'kind' = 'ManagedClusterAddOn') AND ("srchAddon".data->>'name' = 'search-collector')) WHERE (("mcInfo".data->>'kind' = 'ManagedClusterInfo') AND ("srchAddon".uid IS NULL) AND ("mcInfo".data->>'name' != 'local-cluster'))`),
	fmt.Printf("%+v\n", mockRows.mockData)
	// Mock the database query
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "mcInfo".data->>'name' AS "srchAddonDisabledCluster" FROM "search"."resources" AS "mcInfo" LEFT OUTER JOIN "search"."resources" AS "srchAddon" ON (("mcInfo".data->>'name' = "srchAddon".data->>'namespace') AND ("srchAddon".data->>'kind' = 'ManagedClusterAddOn') AND ("srchAddon".data->>'name' = 'search-collector')) WHERE (("mcInfo".data->>'kind' = 'ManagedClusterInfo') AND ("srchAddon".uid IS NULL) AND ("mcInfo".data->>'name' != 'local-cluster'))`),
	).Return(mockRows, fmt.Errorf("err running query"))
	fmt.Println("mockRows.data: ", mockRows.mockData)
	ctx := context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456")
	resolver.buildSearchAddonDisabledQuery(ctx)
	//Execute the function
	res, errRes := resolver.messageResults(ctx)
	fmt.Println("res: ", res)
	fmt.Println("err: ", errRes)

	messages := make([]*model.Message, 0)

	if !reflect.DeepEqual(messages, res) {
		t.Errorf("Message results doesn't match. Expected: %#v, Got: %#v", messages, res)
	}
	if errRes == nil {
		t.Errorf("Incorrect results. expected error to be [%v] got [%v]", fmt.Errorf("err running query"), errRes)
	}
}

func Test_Message_Results_NoDisabledClusters(t *testing.T) {
	csRes, nsRes, _ := newUserData()
	ud := rbac.UserData{CsResources: csRes, NsResources: nsRes, ManagedClusters: map[string]struct{}{}}
	// Create a SearchSchemaResolver instance with a mock connection pool.
	resolver, mockPool := newMockMessage(t, &ud)

	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("../resolver/mocks/mock.json", nil, "srchAddonDisabledCluster", 0)
	// Query before rbac
	// SELECT COUNT(DISTINCT("mcInfo".data->>'name')) FROM "search"."resources" AS "mcInfo" LEFT OUTER JOIN "search"."resources" AS "srchAddon" ON (("mcInfo".data->>'name' = "srchAddon".data->>'namespace') AND ("srchAddon".data->>'kind' = 'ManagedClusterAddOn') AND ("srchAddon".data->>'name' = 'search-collector')) WHERE (("mcInfo".data->>'kind' = 'ManagedClusterInfo') AND ("srchAddon".uid IS NULL) AND ("mcInfo".data->>'name' != 'local-cluster'))`),
	mockRows.mockData = []map[string]interface{}{}
	// Mock the database query
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "mcInfo".data->>'name' AS "srchAddonDisabledCluster" FROM "search"."resources" AS "mcInfo" LEFT OUTER JOIN "search"."resources" AS "srchAddon" ON (("mcInfo".data->>'name' = "srchAddon".data->>'namespace') AND ("srchAddon".data->>'kind' = 'ManagedClusterAddOn') AND ("srchAddon".data->>'name' = 'search-collector')) WHERE (("mcInfo".data->>'kind' = 'ManagedClusterInfo') AND ("srchAddon".uid IS NULL) AND ("mcInfo".data->>'name' != 'local-cluster'))`),
	).Return(mockRows, nil)
	fmt.Println("mockRows.data: ", mockRows.mockData)
	ctx := context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456")
	resolver.buildSearchAddonDisabledQuery(ctx)
	//Execute the function
	res, errRes := resolver.messageResults(ctx)

	messages := make([]*model.Message, 0)

	if !reflect.DeepEqual(messages, res) {
		t.Errorf("Message results doesn't match. Expected: %#v, Got: %#v", messages, res)
	}
	if errRes != nil {
		t.Errorf("Incorrect results. expected error to be [%v] got [%v]", nil, errRes)
	}
}
