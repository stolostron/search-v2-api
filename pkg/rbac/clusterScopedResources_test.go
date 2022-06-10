package rbac

import (
	"context"
	"testing"
	"time"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/golang/mock/gomock"
)

// Initialize cache object to use tests.
func mockResourcesListCache(t *testing.T) (*pgxpoolmock.MockPgxPool, Cache) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)
	return mockPool, Cache{
		shared: clusterScopedResources{},
		pool:   mockPool,
	}
}

func Test_getResources_emptyCache(t *testing.T) {

	ctx := context.Background()
	mockpool, mock_cache := mockResourcesListCache(t)
	columns := []string{"apigroup", "kind"}
	pgxRows := pgxpoolmock.NewRows(columns).AddRow("Node", "addon.open-cluster-management.io").ToPgxRows()

	mockpool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT COALESCE("data"->>'apigroup', '') AS "apigroup", COALESCE("data"->>'kind', '') AS "kind" FROM "search"."resources" WHERE ("cluster"::TEXT = 'local-cluster' AND ("data"->>'namespace' IS NULL))`),
		gomock.Eq([]interface{}{}),
	).Return(pgxRows, nil)

	result, err := mock_cache.ClusterScopedResources(ctx)

	if len(result) == 0 {
		t.Error("Resources not in cache.")
	}
	if err != nil {
		t.Error("Unexpected error while obtaining cluster-scoped resources.", err)
	}
	// Verify that cache was updated within the last 1 millisecond.
	if mock_cache.shared.updatedAt.Before(time.Now().Add(time.Duration(-1) * time.Millisecond)) {
		t.Error("Expected cache.shared.updatedAt to be less than 1 millisecond ago.")
	}

}

func Test_getResouces_usingCache(t *testing.T) {
	ctx := context.Background()
	mockpool, mock_cache := mockResourcesListCache(t)
	columns := []string{"apigroup", "kind"}
	pgxRows := pgxpoolmock.NewRows(columns).AddRow("Node", "addon.open-cluster-management.io").ToPgxRows()

	mockpool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT COALESCE("data"->>'apigroup', '') AS "apigroup", COALESCE("data"->>'kind', '') AS "kind" FROM "search"."resources" WHERE ("cluster"::TEXT = 'local-cluster' AND ("data"->>'namespace' IS NULL))`),
		gomock.Eq([]interface{}{}),
	).Return(pgxRows, nil)

	resourcemap := map[string][]string{"apigroup1": {"kind1", "kind2"}}

	mock_cache.shared = clusterScopedResources{
		updatedAt: time.Now(),
		resources: resourcemap,
	}

	result, err := mock_cache.ClusterScopedResources(ctx)

	if len(result) == 0 {
		t.Error("Expected resources in cache.")
	}

	if err != nil {
		t.Error("Unexpected error while obtaining cluster-scoped resources.", err)
	}
	// Verify that cache was updated within the last 1 millisecond.
	if mock_cache.shared.updatedAt.Before(time.Now().Add(time.Duration(-1) * time.Millisecond)) {
		t.Error("Expected cache.shared.updatedAt to be less than 1 millisecond ago.")
	}
}

func Test_getResources_expiredCache(t *testing.T) {
	ctx := context.Background()
	mockpool, mock_cache := mockResourcesListCache(t)
	columns := []string{"apigroup", "kind"}
	pgxRows := pgxpoolmock.NewRows(columns).AddRow("Node", "addon.open-cluster-management.io").ToPgxRows()

	mockpool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT COALESCE("data"->>'apigroup', '') AS "apigroup", COALESCE("data"->>'kind', '') AS "kind" FROM "search"."resources" WHERE ("cluster"::TEXT = 'local-cluster' AND ("data"->>'namespace' IS NULL))`),
		gomock.Eq([]interface{}{}),
	).Return(pgxRows, nil)

	resourcemap := map[string][]string{"apigroup1": {"kind1", "kind2"}}
	mock_cache.shared = clusterScopedResources{
		updatedAt: time.Now().Add(time.Duration(-3) * time.Minute),
		resources: resourcemap,
	}

	result, err := mock_cache.ClusterScopedResources(ctx)

	if len(result) == 0 {
		t.Error("Resources need to be updated")
	}
	if err != nil {
		t.Error("Unexpected error while obtaining cluster-scoped resources.", err)
	}

}
