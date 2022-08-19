package rbac

import (
	"context"
	"testing"
	"time"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/golang/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakedynclient "k8s.io/client-go/dynamic/fake"
	fakekubeclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

// Initialize cache object to use tests.
func mockResourcesListCache(t *testing.T) (*pgxpoolmock.MockPgxPool, Cache) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)

	testScheme := scheme.Scheme

	err := clusterv1.AddToScheme(testScheme)
	if err != nil {
		t.Errorf("error adding managed cluster scheme: (%v)", err)
	}

	testmc := &clusterv1.ManagedCluster{
		TypeMeta:   metav1.TypeMeta{Kind: "ManagedCluster"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-man"},
	}

	testns := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-namespace", Namespace: "test-namespace"},
	}

	return mockPool, Cache{
		shared:        SharedData{},
		dynamicClient: fakedynclient.NewSimpleDynamicClient(testScheme, testmc),
		restConfig:    &rest.Config{},
		corev1Client:  fakekubeclient.NewSimpleClientset(testns).CoreV1(),
		pool:          mockPool,
	}
}

func Test_getClusterScopedResources_emptyCache(t *testing.T) {

	ctx := context.Background()
	mockpool, mock_cache := mockResourcesListCache(t)
	columns := []string{"kind", "apigroup"}
	pgxRows := pgxpoolmock.NewRows(columns).AddRow("addon.open-cluster-management.io", "Nodes").ToPgxRows()

	mockpool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT COALESCE("data"->>'apigroup', '') AS "apigroup", COALESCE("data"->>'kind_plural', '') AS "kind" FROM "search"."resources" WHERE ("data"->>'_hubClusterResource'='true' AND ("data"->>'namespace' IS NULL))`),
		gomock.Eq([]interface{}{}),
	).Return(pgxRows, nil)

	//namespace

	err := mock_cache.PopulateSharedCache(ctx)

	if len(mock_cache.shared.csResources) != 1 || mock_cache.shared.csResources[0].Kind != "Nodes" ||
		mock_cache.shared.csResources[0].Apigroup != "addon.open-cluster-management.io" {
		t.Error("Cluster Scoped Resources not in cache")
	}

	if len(mock_cache.shared.namespaces) != 1 || mock_cache.shared.namespaces[0] != "test-namespace" {
		t.Error("Shared Namespaces not in cache")
	}

	if len(mock_cache.shared.managedClusters) != 1 || mock_cache.shared.managedClusters[0] != "test-man" {
		t.Error("ManagedClusters not in cache")
	}

	if err != nil {
		t.Error("Unexpected error while obtaining cluster-scoped resources.", err)
	}

}

func Test_getResouces_usingCache(t *testing.T) {
	ctx := context.Background()
	mockpool, mock_cache := mockResourcesListCache(t)
	columns := []string{"apigroup", "kind"}
	pgxRows := pgxpoolmock.NewRows(columns).AddRow("addon.open-cluster-management.io", "Nodes").ToPgxRows()

	mockpool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT COALESCE("data"->>'apigroup', '') AS "apigroup", COALESCE("data"->>'kind_plural', '') AS "kind" FROM "search"."resources" WHERE ("data"->>'_hubClusterResource'='true' AND ("data"->>'namespace' IS NULL))`),
		gomock.Eq([]interface{}{}),
	).Return(pgxRows, nil)

	var managedCluster, namespaces []string

	namespaces = append(namespaces, "test-namespace")
	managedCluster = append(managedCluster, "test-man")

	//Adding cache:
	mock_cache.shared = SharedData{
		namespaces:      namespaces,
		managedClusters: managedCluster,
		mcUpdatedAt:     time.Now(),
		csUpdatedAt:     time.Now(),
		csResources:     append(mock_cache.shared.csResources, Resource{Apigroup: "apigroup1", Kind: "kind1"}),
	}

	err := mock_cache.PopulateSharedCache(ctx)

	if len(mock_cache.shared.csResources) != 1 || mock_cache.shared.csResources[0].Kind != "Nodes" ||
		mock_cache.shared.csResources[0].Apigroup != "addon.open-cluster-management.io" {
		t.Error("Cluster Scoped Resources not in cache")
	}
	if len(mock_cache.shared.namespaces) != 1 || mock_cache.shared.namespaces[0] != "test-namespace" {
		t.Error("Shared Namespaces not in cache")
	}

	if len(mock_cache.shared.managedClusters) != 1 || mock_cache.shared.managedClusters[0] != "test-man" {
		t.Error("ManagedClusters not in cache")
	}

	if err != nil {
		t.Error("Unexpected error while obtaining cluster-scoped resources.", err)
	}

}

func Test_getResources_expiredCache(t *testing.T) {
	ctx := context.Background()
	mockpool, mock_cache := mockResourcesListCache(t)
	columns := []string{"apigroup", "kind"}
	pgxRows := pgxpoolmock.NewRows(columns).AddRow("addon.open-cluster-management.io", "Nodes").ToPgxRows()

	mockpool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT COALESCE("data"->>'apigroup', '') AS "apigroup", COALESCE("data"->>'kind_plural', '') AS "kind" FROM "search"."resources" WHERE ("data"->>'_hubClusterResource'='true' AND ("data"->>'namespace' IS NULL))`),
		gomock.Eq([]interface{}{}),
	).Return(pgxRows, nil)

	var managedCluster, namespaces []string

	namespaces = append(namespaces, "test-namespace")
	managedCluster = append(managedCluster, "test-man")
	//adding expired cache
	last_cache_time := time.Now().Add(time.Duration(-5) * time.Minute)
	mock_cache.shared = SharedData{
		namespaces:      namespaces,
		managedClusters: managedCluster,
		nsUpdatedAt:     last_cache_time,
		mcUpdatedAt:     last_cache_time,
		csUpdatedAt:     last_cache_time,
		csResources:     append(mock_cache.shared.csResources, Resource{Apigroup: "apigroup1", Kind: "kind1"}),
	}

	err := mock_cache.PopulateSharedCache(ctx)

	//
	if len(mock_cache.shared.csResources) != 1 || mock_cache.shared.csResources[0].Kind != "Nodes" ||
		mock_cache.shared.csResources[0].Apigroup != "addon.open-cluster-management.io" {
		t.Error("Cluster Scoped Resources not in cache")
	}

	if len(mock_cache.shared.namespaces) != 1 || mock_cache.shared.namespaces[0] != "test-namespace" {
		t.Error("Shared Namespaces not in cache")
	}

	if len(mock_cache.shared.managedClusters) != 1 || mock_cache.shared.managedClusters[0] != "test-man" {
		t.Error("ManagedClusters not in cache")
	}
	if err != nil {
		t.Error("Unexpected error while obtaining cluster-scoped resources.", err)
	}
	// Verify that cache was updated within the last 2 millisecond.
	if !mock_cache.shared.csUpdatedAt.After(last_cache_time) || !mock_cache.shared.mcUpdatedAt.After(last_cache_time) || !mock_cache.shared.nsUpdatedAt.After(last_cache_time) {
		t.Error("Expected the cache.shared.updatedAt to have a later timestamp")
	}

}
