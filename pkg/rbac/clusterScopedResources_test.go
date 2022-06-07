package rbac

import (
	"testing"
	"time"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/golang/mock/gomock"
)

// Initialize cache object to use tests.
func newResourcesListCache() Cache {
	return Cache{
		pool:   pgxpoolmock.PgxPool(),
		shared: map[string]*sharedList{},
	}
}

func Test_getResources_emptyCache(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)

	cache := newResourcesListCache()

	result, err := cache.checkUserResources("fakeclienttoken", mockpool)

	if result {
		t.Error("Resources not in cache.")
	}
	if err != nil {
		t.Error("Received unexpected error from checkUserResources()", err)
	}

}

func Test_getResouces_usingCache(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)

	cache := newResourcesListCache()
	resourcemap := make(map[string][]string)
	var apigroups string
	var kinds []string

	kinds = append(kinds, "kind1", "kind2")
	apigroups = "apigroup1"

	resourcemap[apigroups] = kinds
	cache.shared["fakeuseruid"] = &sharedList{
		updatedAt: time.Now(),
		resources: resourcemap,
	}

	result, err := cache.checkUserResources("fakeclient", mockPool)

	if result {
		t.Error("Expected resources in cache.")
	}

	if err != nil {
		t.Error("Received unexpected error from checkUserResources()", err)
	}
}

func Test_getResources_expiredCache(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)

	cache := newResourcesListCache()
	resourcemap := make(map[string][]string)
	var apigroups string
	var kinds []string

	kinds = append(kinds, "kind1", "kind2")
	apigroups = "apigroup1"

	resourcemap[apigroups] = kinds
	cache.shared["fakeuseruid"] = &sharedList{
		updatedAt: time.Now().Add(time.Duration(-2) * time.Minute),
		resources: resourcemap,
	}

	result, err := cache.checkUserResources("fakeclient", mockPool)

	if result {
		t.Error("Resources need to be updated")
	}
	if err != nil {
		t.Error("Received unexpected error from checkUserResources()", err)
	}

}
