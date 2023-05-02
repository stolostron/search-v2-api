package rbac

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/golang/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakedynclient "k8s.io/client-go/dynamic/fake"
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

	mockmc := &clusterv1.ManagedCluster{
		TypeMeta:   metav1.TypeMeta{Kind: "ManagedCluster"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-man"},
	}

	mockns := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
	}

	return mockPool, Cache{
		users: map[string]*UserDataCache{},
		shared: SharedData{
			pool:          mockPool,
			dynamicClient: fakedynclient.NewSimpleDynamicClient(testScheme, mockmc, mockns),
		},
		restConfig: &rest.Config{},
		pool:       mockPool,
	}
}

func Test_getClusterScopedResources_emptyCache(t *testing.T) {

	ctx := context.Background()
	mockpool, mock_cache := mockResourcesListCache(t)
	columns := []string{"kind", "apigroup"}
	pgxRows := pgxpoolmock.NewRows(columns).AddRow("addon.open-cluster-management.io", "Nodes").ToPgxRows()

	columns1 := []string{"key", "datatype"}
	pgxRows1 := pgxpoolmock.NewRows(columns1).AddRow("kind", "string").AddRow("apigroup", "string").ToPgxRows()

	mockpool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT COALESCE("data"->>'apigroup', '') AS "apigroup", COALESCE("data"->>'kind_plural', '') AS "kind" FROM "search"."resources" WHERE ("data"->>'_hubClusterResource'='true' AND ("data"->>'namespace' IS NULL))`),
		gomock.Eq([]interface{}{}),
	).Return(pgxRows, nil)

	mockpool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT key, jsonb_typeof("value") AS "datatype" FROM "search"."resources", jsonb_each("data")`),
		gomock.Eq([]interface{}{}),
	).Return(pgxRows1, nil)

	propTypes, _ := mock_cache.GetPropertyTypes(ctx, true) //query map
	propTypes["kind"] = "string"
	propTypes["apigroup"] = "string"

	mock_cache.shared.PopulateSharedCache(ctx)
	res := Resource{Kind: "Nodes", Apigroup: "addon.open-cluster-management.io"}

	_, csResPresent := mock_cache.shared.csResourcesMap[res]
	if len(mock_cache.shared.csResourcesMap) != 1 || !csResPresent {
		t.Error("Cluster Scoped Resources not in cache")
	}

	if len(mock_cache.shared.namespaces) != 1 || mock_cache.shared.namespaces[0] != "test-namespace" || mock_cache.shared.propTypes["kind"] != "string" {
		t.Error("Shared Namespaces not in cache")
	}

	_, ok := mock_cache.shared.managedClusters["test-man"]
	if len(mock_cache.shared.managedClusters) != 1 || !ok {
		t.Error("ManagedClusters not in cache")
	}
}

func Test_getResouces_usingCache(t *testing.T) {
	ctx := context.Background()
	mockpool, mock_cache := mockResourcesListCache(t)
	columns := []string{"apigroup", "kind"}
	pgxRows := pgxpoolmock.NewRows(columns).AddRow("addon.open-cluster-management.io", "Nodes").ToPgxRows()

	columns1 := []string{"key", "datatype"}
	pgxRows1 := pgxpoolmock.NewRows(columns1).AddRow("kind", "string").AddRow("apigroup", "string").ToPgxRows()

	mockpool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT COALESCE("data"->>'apigroup', '') AS "apigroup", COALESCE("data"->>'kind_plural', '') AS "kind" FROM "search"."resources" WHERE ("data"->>'_hubClusterResource'='true' AND ("data"->>'namespace' IS NULL))`),
		gomock.Eq([]interface{}{}),
	).Return(pgxRows, nil)

	mockpool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT key, jsonb_typeof("value") AS "datatype" FROM "search"."resources", jsonb_each("data")`),
		gomock.Eq([]interface{}{}),
	).Return(pgxRows1, nil)

	namespaces := []string{"test-namespace"}
	manClusters := map[string]struct{}{"test-man": {}}
	res := Resource{Apigroup: "apigroup1", Kind: "kind1"}
	csRes := map[Resource]struct{}{}
	propTypesMock := map[string]string{"kind": "string", "label": "object"}

	csRes[res] = struct{}{}
	// Adding cache:
	mock_cache.shared = SharedData{
		namespaces:      namespaces,
		managedClusters: manClusters,
		mcCache:         cacheMetadata{updatedAt: time.Now()},
		csrCache:        cacheMetadata{updatedAt: time.Now()},
		csResourcesMap:  csRes,
		propTypes:       propTypesMock,
		dynamicClient:   mock_cache.shared.dynamicClient,
		pool:            mock_cache.pool,
	}

	mock_cache.shared.PopulateSharedCache(ctx)
	csResource := Resource{Kind: "Nodes", Apigroup: "addon.open-cluster-management.io"}
	_, csResPresent := mock_cache.shared.csResourcesMap[csResource]

	if len(mock_cache.shared.csResourcesMap) != 1 || !csResPresent {
		t.Error("Cluster Scoped Resources not in cache")
	}
	if len(mock_cache.shared.namespaces) != 1 || mock_cache.shared.namespaces[0] != "test-namespace" || mock_cache.shared.propTypes["kind"] != "string" {
		t.Error("Shared Namespaces not in cache")
	}
	_, ok := mock_cache.shared.managedClusters["test-man"]
	if len(mock_cache.shared.managedClusters) != 1 || !ok {
		t.Error("ManagedClusters not in cache")
	}
}

func Test_getResources_expiredCache(t *testing.T) {
	ctx := context.Background()
	mockpool, mock_cache := mockResourcesListCache(t)
	columns := []string{"apigroup", "kind"}
	pgxRows := pgxpoolmock.NewRows(columns).AddRow("addon.open-cluster-management.io", "Nodes").ToPgxRows()

	columns1 := []string{"key", "datatype"}
	pgxRows1 := pgxpoolmock.NewRows(columns1).AddRow("kind", "string").AddRow("apigroup", "string").ToPgxRows()

	mockpool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT COALESCE("data"->>'apigroup', '') AS "apigroup", COALESCE("data"->>'kind_plural', '') AS "kind" FROM "search"."resources" WHERE ("data"->>'_hubClusterResource'='true' AND ("data"->>'namespace' IS NULL))`),
		gomock.Eq([]interface{}{}),
	).Return(pgxRows, nil)

	mockpool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT key, jsonb_typeof("value") AS "datatype" FROM "search"."resources", jsonb_each("data")`),
		gomock.Eq([]interface{}{}),
	).Return(pgxRows1, nil)

	namespaces := []string{"test-namespace"}
	manClusters := map[string]struct{}{"test-man": {}}
	//adding expired cache
	last_cache_time := time.Now().Add(time.Duration(-6) * time.Minute)
	csRes := map[Resource]struct{}{}
	res := Resource{Apigroup: "apigroup1", Kind: "kind1"}

	csRes[res] = struct{}{}
	mock_cache.shared = SharedData{
		namespaces:      namespaces,
		managedClusters: manClusters,
		nsCache:         cacheMetadata{updatedAt: last_cache_time},
		mcCache:         cacheMetadata{updatedAt: last_cache_time},
		csrCache:        cacheMetadata{updatedAt: last_cache_time},
		csResourcesMap:  csRes,
		dynamicClient:   mock_cache.shared.dynamicClient,
		pool:            mock_cache.pool,
	}

	mock_cache.shared.PopulateSharedCache(ctx)

	propTypes, _ := mock_cache.GetPropertyTypes(ctx, false)
	propTypes["kind"] = "string"
	propTypes["apigroup"] = "string"

	csResource := Resource{Kind: "Nodes", Apigroup: "addon.open-cluster-management.io"}
	_, csResPresent := mock_cache.shared.csResourcesMap[csResource]

	if len(mock_cache.shared.csResourcesMap) != 1 || !csResPresent {
		t.Error("Cluster Scoped Resources not in cache")
	}

	if len(mock_cache.shared.namespaces) != 1 || mock_cache.shared.namespaces[0] != "test-namespace" || mock_cache.shared.propTypes["kind"] != "string" {
		t.Error("Shared Namespaces not in cache")
	}
	_, ok := mock_cache.shared.managedClusters["test-man"]
	if len(mock_cache.shared.managedClusters) != 1 || !ok {
		t.Error("ManagedClusters not in cache")
	}
	// Verify that cache was updated within the last 2 millisecond.
	if !mock_cache.shared.csrCache.updatedAt.After(last_cache_time) || !mock_cache.shared.mcCache.updatedAt.After(last_cache_time) || !mock_cache.shared.nsCache.updatedAt.After(last_cache_time) {
		t.Error("Expected the cache.shared.updatedAt to have a later timestamp")
	}

}

func Test_GetandSetDisabledClusters(t *testing.T) {
	_, mock_cache := mockResourcesListCache(t)
	mock_cache.shared.dcCache.updatedAt = time.Now()

	dClusters := map[string]struct{}{"managed1": {}, "managed2": {}}
	setupToken(&mock_cache)
	mock_cache.shared.setDisabledClusters(dClusters, nil)

	//user's managedclusters

	userdataCache := UserDataCache{UserData: UserData{ManagedClusters: dClusters},
		csrCache:      cacheMetadata{updatedAt: time.Now()},
		nsrCache:      cacheMetadata{updatedAt: time.Now()},
		clustersCache: cacheMetadata{updatedAt: time.Now()}}
	setupUserDataCache(&mock_cache, &userdataCache)

	res, _ := mock_cache.GetDisabledClusters(context.WithValue(context.Background(),
		ContextAuthTokenKey, "123456"))
	if len(*res) != 2 {
		t.Errorf("Expected 2 clusters to be in the disabled list %d", len(*res))
	}
}

func Test_setDisabledClusters(t *testing.T) {
	disabledClusters := map[string]struct{}{}
	disabledClusters["disabled1"] = struct{}{}
	_, mock_cache := mockResourcesListCache(t)
	mock_cache.shared.setDisabledClusters(disabledClusters, nil)

	if len(mock_cache.shared.disabledClusters) != 1 || mock_cache.shared.dcCache.err != nil {
		t.Error("Expected the cache.shared.disabledClusters to be updated with 1 cluster and no error")
	}
}

//ContextAuthTokenKey is not set - so session info cannot be found
func Test_getDisabledClusters_UserNotFound(t *testing.T) {
	disabledClusters := map[string]struct{}{}
	disabledClusters["disabled1"] = struct{}{}
	_, mock_cache := mockResourcesListCache(t)
	mock_cache.tokenReviews = map[string]*tokenReviewCache{}
	//user's managedclusters
	manClusters := map[string]struct{}{}
	manClusters["disabled1"] = struct{}{}
	userdataCache := UserDataCache{UserData: UserData{ManagedClusters: manClusters},
		csrCache:      cacheMetadata{updatedAt: time.Now()},
		nsrCache:      cacheMetadata{updatedAt: time.Now()},
		clustersCache: cacheMetadata{updatedAt: time.Now()}}
	setupUserDataCache(&mock_cache, &userdataCache)

	mock_cache.shared.dcCache.err = nil
	mock_cache.shared.disabledClusters = disabledClusters
	mock_cache.shared.dcCache.updatedAt = time.Now()

	// Context key is not set - so, user won't be found
	disabledClustersRes, err := mock_cache.GetDisabledClusters(context.TODO())

	if disabledClustersRes != nil || err == nil {
		t.Error("Expected the cache.shared.disabledClusters to be nil and to have error")
	}
}

//disabled cluster cache is valid
func Test_getDisabledClustersValid(t *testing.T) {
	disabledClusters := map[string]struct{}{}
	disabledClusters["disabled1"] = struct{}{}
	_, mock_cache := mockResourcesListCache(t)

	setupToken(&mock_cache)
	//user's managedclusters
	manClusters := map[string]struct{}{"disabled1": {}}

	userdataCache := UserDataCache{UserData: UserData{ManagedClusters: manClusters},
		csrCache:      cacheMetadata{updatedAt: time.Now()},
		nsrCache:      cacheMetadata{updatedAt: time.Now()},
		clustersCache: cacheMetadata{updatedAt: time.Now()}}
	setupUserDataCache(&mock_cache, &userdataCache)

	mock_cache.shared.dcCache.err = nil
	mock_cache.shared.disabledClusters = disabledClusters
	mock_cache.shared.dcCache.updatedAt = time.Now()

	disabledClustersRes, err := mock_cache.GetDisabledClusters(context.WithValue(context.Background(),
		ContextAuthTokenKey, "123456"))

	if len(*disabledClustersRes) != 1 || err != nil {
		t.Error("Expected the cache.shared.disabledClusters to be valid/updated with 1 cluster and to have no error")
	}
}

//user does not have access to disabled managed clusters
func Test_getDisabledClustersValid_User_NoAccess(t *testing.T) {
	disabledClusters := map[string]struct{}{}
	disabledClusters["disabled1"] = struct{}{}
	_, mock_cache := mockResourcesListCache(t)
	setupToken(&mock_cache)
	//user's managedclusters
	manClusters := map[string]struct{}{"managed1": {}}

	//user only has access to "managed1" cluster, but not "disabled1" cluster
	userdataCache := UserDataCache{UserData: UserData{ManagedClusters: manClusters},
		csrCache:      cacheMetadata{updatedAt: time.Now()},
		nsrCache:      cacheMetadata{updatedAt: time.Now()},
		clustersCache: cacheMetadata{updatedAt: time.Now()}}
	setupUserDataCache(&mock_cache, &userdataCache)

	mock_cache.shared.dcCache.err = nil
	mock_cache.shared.disabledClusters = disabledClusters
	mock_cache.shared.dcCache.updatedAt = time.Now()

	disabledClustersRes, err := mock_cache.GetDisabledClusters(context.WithValue(context.Background(),
		ContextAuthTokenKey, "123456"))

	if len(*disabledClustersRes) != 0 || err != nil {
		t.Error("Expected the cache.shared.disabledClusters to be updated with 1 cluster and to have no error")
	}
}

//cache is invalid. So, run the db query and get results
func Test_getDisabledClustersCacheInValid_RunQuery(t *testing.T) {
	disabledClusters := map[string]struct{}{}
	disabledClusters["disabled1"] = struct{}{}
	_, mock_cache := mockResourcesListCache(t)
	setupToken(&mock_cache)

	//user's managedclusters
	manClusters := map[string]struct{}{"disabled1": {}}
	userdataCache := UserDataCache{UserData: UserData{ManagedClusters: manClusters},
		csrCache:      cacheMetadata{updatedAt: time.Now()},
		nsrCache:      cacheMetadata{updatedAt: time.Now()},
		clustersCache: cacheMetadata{updatedAt: time.Now()}}
	setupUserDataCache(&mock_cache, &userdataCache)

	mock_cache.shared.dcCache.err = nil
	mock_cache.shared.disabledClusters = disabledClusters
	mock_cache.shared.dcCache.updatedAt = time.Now().Add(-24 * time.Hour) //to invalidate cache

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)
	disabledClustersRows := map[string]interface{}{}
	disabledClustersRows["disabled1"] = ""

	pgxRows := pgxpoolmock.NewRows([]string{"srchAddonDisabledCluster"}).AddRow("disabled1").ToPgxRows()

	// Mock the database query
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "mcInfo".data->>'name' AS "srchAddonDisabledCluster" FROM "search"."resources" AS "mcInfo" LEFT OUTER JOIN "search"."resources" AS "srchAddon" ON (("mcInfo".data->>'name' = "srchAddon".data->>'namespace') AND ("srchAddon".data->>'kind' = 'ManagedClusterAddOn') AND ("srchAddon".data->>'name' = 'search-collector')) WHERE (("mcInfo".data->>'kind' = 'ManagedClusterInfo') AND ("srchAddon".uid IS NULL) AND ("mcInfo".data->>'name' != 'local-cluster'))`),
	).Return(pgxRows, nil)

	mock_cache.pool = mockPool
	mock_cache.shared.pool = mockPool
	disabledClustersRes, err := mock_cache.GetDisabledClusters(context.WithValue(context.Background(), ContextAuthTokenKey, "123456"))
	if len(*disabledClustersRes) != 1 || err != nil {
		t.Error("Expected the cache.shared.disabledClusters to be updated with 1 cluster and no error")
	}
}

//cache is invalid. So, run the db query - error while running the query
func Test_getDisabledClustersCacheInValid_RunQueryError(t *testing.T) {
	disabledClusters := map[string]struct{}{}
	disabledClusters["disabled1"] = struct{}{}
	_, mock_cache := mockResourcesListCache(t)
	setupToken(&mock_cache)

	//user's managedclusters
	manClusters := map[string]struct{}{"managed1": {}}

	//user has no access to disabled clusters
	userdataCache := UserDataCache{UserData: UserData{ManagedClusters: manClusters},
		csrCache:      cacheMetadata{updatedAt: time.Now()},
		nsrCache:      cacheMetadata{updatedAt: time.Now()},
		clustersCache: cacheMetadata{updatedAt: time.Now()}}
	setupUserDataCache(&mock_cache, &userdataCache)

	mock_cache.shared.dcCache.err = nil
	mock_cache.shared.disabledClusters = disabledClusters
	mock_cache.shared.dcCache.updatedAt = time.Now().Add(-24 * time.Hour) // to invalidate cache

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)
	disabledClustersRows := map[string]interface{}{}
	disabledClustersRows["disabled1"] = ""

	pgxRows := pgxpoolmock.NewRows([]string{"srchAddonDisabledCluster"}).AddRow("disabled1").ToPgxRows()

	// Mock the database query
	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq(`SELECT DISTINCT "mcInfo".data->>'name' AS "srchAddonDisabledCluster" FROM "search"."resources" AS "mcInfo" LEFT OUTER JOIN "search"."resources" AS "srchAddon" ON (("mcInfo".data->>'name' = "srchAddon".data->>'namespace') AND ("srchAddon".data->>'kind' = 'ManagedClusterAddOn') AND ("srchAddon".data->>'name' = 'search-collector')) WHERE (("mcInfo".data->>'kind' = 'ManagedClusterInfo') AND ("srchAddon".uid IS NULL) AND ("mcInfo".data->>'name' != 'local-cluster'))`),
	).Return(pgxRows, fmt.Errorf("Error fetching data"))
	mock_cache.pool = mockPool
	mock_cache.shared.pool = mockPool
	disabledClustersRes, err := mock_cache.GetDisabledClusters(context.WithValue(context.Background(), ContextAuthTokenKey, "123456"))

	if disabledClustersRes != nil || err == nil {
		t.Error("Expected the cache.shared.disabledClusters to have error fetchng data")
	}
}

func Test_Messages_Query(t *testing.T) {

	sql := `SELECT DISTINCT "mcInfo".data->>'name' AS "srchAddonDisabledCluster" FROM "search"."resources" AS "mcInfo" LEFT OUTER JOIN "search"."resources" AS "srchAddon" ON (("mcInfo".data->>'name' = "srchAddon".data->>'namespace') AND ("srchAddon".data->>'kind' = 'ManagedClusterAddOn') AND ("srchAddon".data->>'name' = 'search-collector')) WHERE (("mcInfo".data->>'kind' = 'ManagedClusterInfo') AND ("srchAddon".uid IS NULL) AND ("mcInfo".data->>'name' != 'local-cluster'))`
	// Execute function
	query, err := buildSearchAddonDisabledQuery(context.WithValue(context.Background(), ContextAuthTokenKey, "123456"))

	// Verify response
	if query != sql {
		t.Errorf("Expected sql query: %s but got %s", sql, query)
	}
	if err != nil {
		t.Errorf("Expected error to be nil, but got : %s", err)
	}
}
