// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"reflect"
	"testing"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/golang/mock/gomock"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	fake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

// Initialize cache object to use tests.
func MockResourcesListCache(t *testing.T) (*pgxpoolmock.MockPgxPool, *rbac.Cache) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)

	testScheme := scheme.Scheme

	err := clusterv1.AddToScheme(testScheme)
	if err != nil {
		t.Errorf("error adding managed cluster scheme: (%v)", err)
	}

	// testmc := &clusterv1.ManagedCluster{
	// 	TypeMeta:   metav1.TypeMeta{Kind: "ManagedCluster"},
	// 	ObjectMeta: metav1.ObjectMeta{Name: "test-man"},
	// }

	// testns := &corev1.Namespace{
	// 	TypeMeta:   metav1.TypeMeta{Kind: "Namespace"},
	// 	ObjectMeta: metav1.ObjectMeta{Name: "test-namespace", Namespace: "test-namespace"},
	// }
	c := &rbac.Cache{
		// users:         map[string]*rbac.UserDataCache{},
		// shared:        SharedData{},
		// dynamicClient: fakedynclient.NewSimpleDynamicClient(testScheme, testmc),
		RestConfig: &rest.Config{},
		// tokenReviews:     map[string]*tokenReviewCache{},
		// corev1Client: fakekubeclient.NewSimpleClientset(testns).CoreV1(),
		Pool: mockPool,
	}
	trc := rbac.TokenReviewCache{AuthClient: fake.NewSimpleClientset().AuthenticationV1()}
	tokenReviews := map[string]*rbac.TokenReviewCache{}
	tokenReviews["123456"] = &trc
	c.SetTokenReviews(tokenReviews)
	return mockPool, c

}

func Test_Message_Results(t *testing.T) {
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

	ctx := context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456")
	//Execute the function
	rbac.CacheInst = rbac.Cache{Pool: mockPool}

	res, err := resolver.messageResults(ctx) //&rbac.Cache{Pool: mockPool}

	messages := make([]*model.Message, 0)
	// kind := "information"
	// desc := "Search is disabled on some of your managed clusters."
	// message := model.Message{ID: "S20",
	// 	Kind:        &kind,
	// 	Description: &desc}
	// messages = append(messages, &message)

	if !reflect.DeepEqual(messages, res) {
		t.Errorf("Message results doesn't match. Expected: %#v, Got: %#v", messages, res)
	}
	if err != nil {
		t.Errorf("Incorrect results. expected error to be [%v] got [%v]", nil, err)
	}
}

func Test_Messages(t *testing.T) {
	csRes, nsRes, mc := newUserData()
	ud := rbac.UserData{CsResources: csRes, NsResources: nsRes, ManagedClusters: mc}

	// Create a SearchSchemaResolver instance with a mock connection pool.
	_, mockPool := newMockMessage(t, &ud)

	// Mock the database queries.
	mockRows := newMockRowsWithoutRBAC("../resolver/mocks/mock.json", nil, "srchAddonDisabledCluster", 0)

	// Mock the database query
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "mcInfo".data->>'name' AS "srchAddonDisabledCluster" FROM "search"."resources" AS "mcInfo" LEFT OUTER JOIN "search"."resources" AS "srchAddon" ON (("mcInfo".data->>'name' = "srchAddon".data->>'namespace') AND ("srchAddon".data->>'kind' = 'ManagedClusterAddOn') AND ("srchAddon".data->>'name' = 'search-collector')) WHERE (("mcInfo".data->>'kind' = 'ManagedClusterInfo') AND ("srchAddon".uid IS NULL) AND ("mcInfo".data->>'name' != 'local-cluster'))`),
	).Return(mockRows, nil)

	ctx := context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456")
	//Execute the function
	rbac.CacheInst = rbac.Cache{Pool: mockPool}
	MockResourcesListCache(t)
	// dynamicClient: fakedynclient.NewSimpleDynamicClient(testScheme, testmc),
	// restConfig:    &rest.Config{},
	// corev1Client:  fakekubeclient.NewSimpleClientset(testns).CoreV1(),
	res, err := Messages(ctx)

	messages := make([]*model.Message, 0)

	if !reflect.DeepEqual(messages, res) {
		t.Errorf("Message results doesn't match. Expected: %#v, Got: %#v", messages, res)
	}
	if err != nil {
		t.Errorf("Incorrect results. expected error to be [%v] got [%v]", nil, err)
	}
}
