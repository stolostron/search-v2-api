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
	res := Resource{Kind: "Nodes", Apigroup: "addon.open-cluster-management.io"}
	_, csResPresent := mock_cache.shared.csResourcesMap[res]
	if len(mock_cache.shared.csResourcesMap) != 1 || !csResPresent {
		t.Error("Cluster Scoped Resources not in cache")
	}

	if len(mock_cache.shared.namespaces) != 1 || mock_cache.shared.namespaces[0] != "test-namespace" {
		t.Error("Shared Namespaces not in cache")
	}

	_, ok := mock_cache.shared.managedClusters["test-man"]
	if len(mock_cache.shared.managedClusters) != 1 || !ok {
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

	var namespaces []string
	namespaces = append(namespaces, "test-namespace")
	managedCluster := make(map[string]struct{})
	managedCluster["test-man"] = struct{}{}
	res := Resource{Apigroup: "apigroup1", Kind: "kind1"}
	csRes := map[Resource]struct{}{}

	csRes[res] = struct{}{}
	//Adding cache:
	mock_cache.shared = SharedData{
		namespaces:      namespaces,
		managedClusters: managedCluster,
		mcUpdatedAt:     time.Now(),
		csUpdatedAt:     time.Now(),
		csResourcesMap:  csRes,
	}

	err := mock_cache.PopulateSharedCache(ctx)
	csResource := Resource{Kind: "Nodes", Apigroup: "addon.open-cluster-management.io"}
	_, csResPresent := mock_cache.shared.csResourcesMap[csResource]

	if len(mock_cache.shared.csResourcesMap) != 1 || !csResPresent {
		t.Error("Cluster Scoped Resources not in cache")
	}
	if len(mock_cache.shared.namespaces) != 1 || mock_cache.shared.namespaces[0] != "test-namespace" {
		t.Error("Shared Namespaces not in cache")
	}
	_, ok := mock_cache.shared.managedClusters["test-man"]
	if len(mock_cache.shared.managedClusters) != 1 || !ok {
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

	var namespaces []string

	namespaces = append(namespaces, "test-namespace")
	managedCluster := make(map[string]struct{})
	managedCluster["test-man"] = struct{}{}
	//adding expired cache
	last_cache_time := time.Now().Add(time.Duration(-5) * time.Minute)
	csRes := map[Resource]struct{}{}
	res := Resource{Apigroup: "apigroup1", Kind: "kind1"}

	csRes[res] = struct{}{}
	mock_cache.shared = SharedData{
		namespaces:      namespaces,
		managedClusters: managedCluster,
		nsUpdatedAt:     last_cache_time,
		mcUpdatedAt:     last_cache_time,
		csUpdatedAt:     last_cache_time,
		csResourcesMap:  csRes,
	}

	err := mock_cache.PopulateSharedCache(ctx)

	csResource := Resource{Kind: "Nodes", Apigroup: "addon.open-cluster-management.io"}
	_, csResPresent := mock_cache.shared.csResourcesMap[csResource]

	if len(mock_cache.shared.csResourcesMap) != 1 || !csResPresent {
		t.Error("Cluster Scoped Resources not in cache")
	}

	if len(mock_cache.shared.namespaces) != 1 || mock_cache.shared.namespaces[0] != "test-namespace" {
		t.Error("Shared Namespaces not in cache")
	}
	_, ok := mock_cache.shared.managedClusters["test-man"]
	if len(mock_cache.shared.managedClusters) != 1 || !ok {
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

func Test_SharedCacheDisabledClustersInValid(t *testing.T) {
	_, mock_cache := mockResourcesListCache(t)
	valid := mock_cache.SharedCacheDisabledClustersValid()
	if valid {
		t.Errorf("Expected false from cache validity check. Got %t", valid)
	}
}

func Test_SharedCacheDisabledClustersValid(t *testing.T) {
	_, mock_cache := mockResourcesListCache(t)
	mock_cache.shared.dcUpdatedAt = time.Now()
	valid := mock_cache.SharedCacheDisabledClustersValid()
	if !valid {
		t.Errorf("Expected false from cache validity check. Got %t", valid)
	}
}

func Test_GetandSetDisabledClusters(t *testing.T) {
	_, mock_cache := mockResourcesListCache(t)
	mock_cache.shared.dcUpdatedAt = time.Now()
	dClusters := make(map[string]struct{})
	dClusters["managed1"] = struct{}{}
	dClusters["managed2"] = struct{}{}

	// mock_cache.shared.disabledClusters = dClusters
	mock_cache.SetDisabledClusters(dClusters, nil)
	res := mock_cache.GetDisabledClusters()
	if len(*res) != 2 {
		t.Errorf("Expected 2 clusters to be in the disabled list %d", len(*res))
	}
}
