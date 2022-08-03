package rbac

import (
	"context"
	"testing"
	"time"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/golang/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakedynclient "k8s.io/client-go/dynamic/fake"
	fakekubeclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

// Initialize cache object to use tests.
func mockResourcesListCache(t *testing.T) (*pgxpoolmock.MockPgxPool, Cache) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)

	var scheme *runtime.Scheme = runtime.NewScheme()

	gvr := schema.GroupVersionResource{Group: "cluster.open-cluster-management.io", Version: "v1", Resource: "managedcluster"}
	gvk := gvr.GroupVersion().WithKind("ManagedCluster")
	listGVK := gvk
	listGVK.Kind += "List"

	r := mockResource{}
	r.SetGroupVersionKind(gvk)

	scheme.AddKnownTypeWithName(gvk, &mockResource{})
	scheme.AddKnownTypeWithName(listGVK, &mockResourceList{})

	return mockPool, Cache{
		shared:        SharedData{},
		dynamicClient: fakedynclient.NewSimpleDynamicClient(scheme, &r),
		restConfig:    &rest.Config{},
		corev1Client:  fakekubeclient.NewSimpleClientset().CoreV1(),
		pool:          mockPool,
	}
}

func Test_getResources_emptyCache(t *testing.T) {

	ctx := context.Background()
	mockpool, mock_cache := mockResourcesListCache(t)
	columns := []string{"apigroup", "kind"}
	pgxRows := pgxpoolmock.NewRows(columns).AddRow("Node", "addon.open-cluster-management.io").ToPgxRows()

	mockpool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT COALESCE("data"->>'apigroup', '') AS "apigroup", COALESCE("data"->>'kind_plural', '') AS "kind" FROM "search"."resources" WHERE ("cluster"::TEXT = 'local-cluster' AND ("data"->>'namespace' IS NULL))`),
		gomock.Eq([]interface{}{}),
	).Return(pgxRows, nil)

	err := mock_cache.PopulateSharedCache(ctx)

	if len(mock_cache.shared.csResources) == 0 {
		t.Error("Resources not in cache.")
	}
	if err != nil {
		t.Error("Unexpected error while obtaining cluster-scoped resources.", err)
	}

}

func Test_getResouces_usingCache(t *testing.T) {
	ctx := context.Background()
	mockpool, mock_cache := mockResourcesListCache(t)
	columns := []string{"apigroup", "kind"}
	pgxRows := pgxpoolmock.NewRows(columns).AddRow("Node", "addon.open-cluster-management.io").ToPgxRows()

	mockpool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT COALESCE("data"->>'apigroup', '') AS "apigroup", COALESCE("data"->>'kind_plural', '') AS "kind" FROM "search"."resources" WHERE ("cluster"::TEXT = 'local-cluster' AND ("data"->>'namespace' IS NULL))`),
		gomock.Eq([]interface{}{}),
	).Return(pgxRows, nil)

	//Adding cache:
	mock_cache.shared = SharedData{
		csUpdatedAt: time.Now(),
		csResources: append(mock_cache.shared.csResources, resource{apigroup: "apigroup1", kind: "kind1"}),
	}

	err := mock_cache.PopulateSharedCache(ctx)

	if len(mock_cache.shared.csResources) == 0 {
		t.Error("Expected resources in cache.")
	}

	if err != nil {
		t.Error("Unexpected error while obtaining cluster-scoped resources.", err)
	}

}

func Test_getResources_expiredCache(t *testing.T) {
	ctx := context.Background()
	mockpool, mock_cache := mockResourcesListCache(t)
	columns := []string{"apigroup", "kind"}
	pgxRows := pgxpoolmock.NewRows(columns).AddRow("Node", "addon.open-cluster-management.io").ToPgxRows()

	mockpool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT COALESCE("data"->>'apigroup', '') AS "apigroup", COALESCE("data"->>'kind_plural', '') AS "kind" FROM "search"."resources" WHERE ("cluster"::TEXT = 'local-cluster' AND ("data"->>'namespace' IS NULL))`),
		gomock.Eq([]interface{}{}),
	).Return(pgxRows, nil)

	//adding expired cache
	last_cache_time := time.Now().Add(time.Duration(-5) * time.Minute)
	mock_cache.shared = SharedData{
		csUpdatedAt: last_cache_time,
		csResources: append(mock_cache.shared.csResources, resource{apigroup: "apigroup1", kind: "kind1"}),
	}

	err := mock_cache.PopulateSharedCache(ctx)

	//
	if len(mock_cache.shared.csResources) == 0 {
		t.Error("Resources need to be updated")
	}
	if err != nil {
		t.Error("Unexpected error while obtaining cluster-scoped resources.", err)
	}
	// Verify that cache was updated within the last 2 millisecond.
	if !mock_cache.shared.csUpdatedAt.After(last_cache_time) {
		t.Error("Expected the cache.shared.updatedAt to have a later timestamp")
	}

}

func Test_getManagedClusters_usingCache(t *testing.T) {
	ctx := context.Background()
	mockpool, mock_cache := mockResourcesListCache(t)
	columns := []string{"apigroup", "kind"}
	pgxRows := pgxpoolmock.NewRows(columns).AddRow("Node", "addon.open-cluster-management.io").ToPgxRows()

	mockpool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT COALESCE("data"->>'apigroup', '') AS "apigroup", COALESCE("data"->>'kind_plural', '') AS "kind" FROM "search"."resources" WHERE ("cluster"::TEXT = 'local-cluster' AND ("data"->>'namespace' IS NULL))`),
		gomock.Eq([]interface{}{}),
	).Return(pgxRows, nil)

	var managedClusterList []string
	managedClusterList = append(managedClusterList, "some-managed-cluster")

	//Adding cache:
	mock_cache.shared = SharedData{
		mcUpdatedAt:     time.Now(),
		managedClusters: managedClusterList,
	}

	err := mock_cache.PopulateSharedCache(ctx)

	if len(mock_cache.shared.csResources) == 0 {
		t.Error("Expected resources in cache.")
	}

	if err != nil {
		t.Error("Unexpected error while obtaining cluster-scoped resources.", err)
	}

}

// https://github.com/kubernetes/client-go/blob/68639ba114e2ca8ad0994ecbddd0ea2c6b8d97c8/dynamic/fake/simple_test.go#L445-L469
type (
	mockResource struct {
		metav1.TypeMeta   `json:",inline"`
		metav1.ObjectMeta `json:"metadata"`
	}
	mockResourceList struct {
		metav1.TypeMeta `json:",inline"`
		metav1.ListMeta `json:"metadata"`

		Items []mockResource
	}
)

func (l *mockResourceList) DeepCopyObject() runtime.Object {
	o := *l
	return &o
}

func (r *mockResource) DeepCopyObject() runtime.Object {
	o := *r
	return &o
}

var _ runtime.Object = (*mockResource)(nil)
var _ runtime.Object = (*mockResourceList)(nil)
