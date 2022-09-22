// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/stolostron/search-v2-api/graph/model"
)

func Test_Messages_ValidCache(t *testing.T) {
	// Build mock
	mockMessage := Message{
		cache: &MockCache{
			disabled: map[string]struct{}{"managed1": {}},
		},
	}
	//Execute the function
	res, err := mockMessage.messageResults(context.Background())

	// Validate
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

//no uid set in context - returns error
func Test_Messages_Error(t *testing.T) {
	// Build mock
	mockMessage := Message{
		cache: &MockCache{
			disabled: map[string]struct{}{"managed1": {}},
			err:      fmt.Errorf("err running query"),
		},
	}
	//Execute the function
	res, errRes := mockMessage.messageResults(context.Background()) //no uid set in context - returns error

	// Validate
	if !reflect.DeepEqual([]*model.Message{}, res) {
		t.Errorf("Message results doesn't match. Expected: %#v, Got: %#v", []*model.Message{}, res)
	}
	if errRes == nil {
		t.Errorf("Incorrect results. expected error to be [%v] got [%v]", fmt.Errorf("err running query"), errRes)
	}
}

func Test_Message_Results_ValidCache(t *testing.T) {
	// Build mock
	mockMessage := Message{
		cache: &MockCache{
			disabled: map[string]struct{}{"managed1": {}},
			err:      nil,
		},
	}
	//Execute the function
	res, err := mockMessage.messageResults(context.Background())

	// Validate
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

// user does not have access to disabled clusters
func Test_Message_Results_NoAccessToDisabledC(t *testing.T) {
	// Build mock
	mockMessage := Message{
		cache: &MockCache{
			disabled: map[string]struct{}{},
			err:      nil,
		},
	}
	//Execute the function
	res, err := mockMessage.messageResults(context.Background())

	// Validate
	if !reflect.DeepEqual([]*model.Message{}, res) {
		t.Errorf("Message results doesn't match. Expected: %#v, Got: %#v", []*model.Message{}, res)
	}
	if err != nil {
		t.Errorf("Incorrect results. expected error to be [%v] got [%v]", nil, err)
	}
}

//Return error running query
// func Test_Message_Results_ErrRunningQuery(t *testing.T) {
// 	csRes, nsRes, mc := newUserData()
// 	ud := rbac.UserData{CsResources: csRes, NsResources: nsRes, ManagedClusters: mc}
// 	// rbac.CacheInst.SetDisabledClusters(map[string]struct{}{}, fmt.Errorf("error"))
// 	// Create a SearchSchemaResolver instance with a mock connection pool.
// 	resolver, mockPool := newMockMessage(t, &ud)
// 	rbac.CacheInst = *rbac.NewMockCacheForMessages(nil, nil, mockPool)

// 	// Mock the database queries.
// 	mockRows := newMockRowsWithoutRBAC("../resolver/mocks/mock.json", nil, "srchAddonDisabledCluster", 0)

// 	// Mock the database query - return error
// 	mockPool.EXPECT().Query(gomock.Any(),
// 		gomock.Eq(`SELECT DISTINCT "mcInfo".data->>'name' AS "srchAddonDisabledCluster" FROM "search"."resources" AS "mcInfo" LEFT OUTER JOIN "search"."resources" AS "srchAddon" ON (("mcInfo".data->>'name' = "srchAddon".data->>'namespace') AND ("srchAddon".data->>'kind' = 'ManagedClusterAddOn') AND ("srchAddon".data->>'name' = 'search-collector')) WHERE (("mcInfo".data->>'kind' = 'ManagedClusterInfo') AND ("srchAddon".uid IS NULL) AND ("mcInfo".data->>'name' != 'local-cluster'))`),
// 	).Return(mockRows, fmt.Errorf("err running query"))

// 	ctx := context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456")

// 	//Execute the function
// 	res, errRes := resolver.messageResults(ctx)

// 	messages := make([]*model.Message, 0)

// 	if !reflect.DeepEqual(messages, res) {
// 		t.Errorf("Message results doesn't match. Expected: %#v, Got: %#v", messages, res)
// 	}
// 	if errRes == nil {
// 		t.Errorf("Incorrect results. expected error to be [%v] got [%v]", fmt.Errorf("err running query"), errRes)
// 	}
// }
