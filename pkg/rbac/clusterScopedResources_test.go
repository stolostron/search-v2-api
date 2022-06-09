package rbac

import (
	"testing"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/golang/mock/gomock"
)

// Initialize cache object to use tests.
func newResourcesListCache(t *testing.T) (*pgxpoolmock.MockPgxPool, Cache) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)
	return mockPool, Cache{
		shared: sharedList{},
		pool:   mockPool,
	}
}

//from test cache:  var mockPool *pgxpoolmock.MockPgxPool
//mockpool.EXPECT undefined (type *pgxpoolmock.PgxPool has no field or method EXPECT)

func Test_getResources_emptyCache(t *testing.T) {
	// ctrl := gomock.NewController(t)
	// defer ctrl.Finish()
	// mockPool := pgxpoolmock.NewMockPgxPool(ctrl)

	// database.NewDAO(mockPool)
	mockpool, cache := newResourcesListCache(t)

	mockpool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT COALESCE("data"->>'apigroup', '') AS "apigroup", COALESCE("data"->>'kind', '') AS "kind" FROM "search"."resources" WHERE ("cluster"::TEXT = 'local-cluster' AND ("data"->>'namespace' IS NULL))])`),
		gomock.Eq([]interface{}{}),
	)
	// .Return(mockRows, nil)

	result, err := cache.checkUserResources()

	if len(result.resources) != 0 {
		t.Error("Resources not in cache.")
	}
	if err != nil {
		t.Error("Unexpected error while obtaining cluster-scoped resources.", err)
	}

}

// func Test_getResouces_usingCache(t *testing.T) {
// 	ctrl := gomock.NewController(t)
// 	defer ctrl.Finish()
// 	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)
// 	database.NewDAO(mockPool)

// 	cache := newResourcesListCache(t)
// 	resourcemap := make(map[string][]string)
// 	var apigroups string
// 	var kinds []string

// 	kinds = append(kinds, "kind1", "kind2")
// 	apigroups = "apigroup1"

// 	resourcemap[apigroups] = kinds
// 	cache.shared = sharedList{
// 		updatedAt: time.Now(),
// 		resources: resourcemap,
// 	}

// 	result, err := cache.checkUserResources()

// 	if len(result.resources) != 0 {
// 		t.Error("Expected resources in cache.")
// 	}

// 	if err != nil {
// 		t.Error("Received unexpected error from checkUserResources()", err)
// 	}
// }

// func Test_getResources_expiredCache(t *testing.T) {
// 	ctrl := gomock.NewController(t)
// 	defer ctrl.Finish()
// 	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)
// 	database.NewDAO(mockPool)

// 	cache := newResourcesListCache(t)
// 	resourcemap := make(map[string][]string)
// 	var apigroups string
// 	var kinds []string

// 	kinds = append(kinds, "kind1", "kind2")
// 	apigroups = "apigroup1"

// 	resourcemap[apigroups] = kinds
// 	cache.shared = sharedList{
// 		updatedAt: time.Now().Add(time.Duration(-2) * time.Minute),
// 		resources: resourcemap,
// 	}

// 	result, err := cache.checkUserResources()

// 	if len(result.resources) != 0 {
// 		t.Error("Resources need to be updated")
// 	}
// 	if err != nil {
// 		t.Error("Received unexpected error from checkUserResources()", err)
// 	}

// }
