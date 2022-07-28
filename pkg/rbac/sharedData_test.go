package rbac

import (
	"context"
	"testing"
	"time"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/golang/mock/gomock"
	fakedynclient "k8s.io/client-go/dynamic/fake"
	fakekubeclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

// Initialize cache object to use tests.
func mockResourcesListCache(t *testing.T) (*pgxpoolmock.MockPgxPool, Cache) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)
	return mockPool, Cache{
		shared:        SharedData{},
		dynamicConfig: &fakedynclient.FakeDynamicClient{},
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

	result, err := mock_cache.ClusterScopedResources(ctx)

	if len(result) == 0 {
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

	result, err := mock_cache.ClusterScopedResources(ctx)

	if len(result) == 0 {
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
	mock_cache.shared = SharedData{
		csUpdatedAt: time.Now().Add(time.Duration(-5) * time.Minute),
		csResources: append(mock_cache.shared.csResources, resource{apigroup: "apigroup1", kind: "kind1"}),
	}

	result, err := mock_cache.ClusterScopedResources(ctx)

	if len(result) == 0 {
		t.Error("Resources need to be updated")
	}
	if err != nil {
		t.Error("Unexpected error while obtaining cluster-scoped resources.", err)
	}
	// Verify that cache was updated within the last 2 millisecond.
	if mock_cache.shared.csUpdatedAt.After(time.Now().Add(time.Duration(-2) * time.Millisecond)) {
		t.Error("Expected the cached cluster scoped resources to be updated within the last 2 milliseconds.")
	}

}
