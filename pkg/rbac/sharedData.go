package rbac

import (
	"context"
	"sync"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/stolostron/search-v2-api/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

// Cache data shared across all users.
type SharedData struct {
	// These are the data fields.
	csResourcesMap   map[Resource]struct{}
	namespaces       []string
	managedClusters  map[string]struct{}
	disabledClusters map[string]struct{}
	propTypes        map[string]string

	// These are internal objects to track the state of the cache.
	dcErr       error      // Error while updating clusters data.
	dcLock      sync.Mutex // Locks when clusters data is being updated.
	dcUpdatedAt time.Time  // Time clusters was last updated.

	mcErr       error      // Error while updating clusters data.
	mcLock      sync.Mutex // Locks when clusters data is being updated.
	mcUpdatedAt time.Time  // Time clusters was last updated.

	csErr       error      // Capture errors retrieving cluster-scoped resources.
	csLock      sync.Mutex // Locks the csResources map while updating it.
	csUpdatedAt time.Time  // Time when cluster-scoped data was last updated.

	nsErr       error      // Capture errors retrieving namespaces.
	nsLock      sync.Mutex // Locks the namespaces array while updating it.
	nsUpdatedAt time.Time  // Time when namespaces data was last updated.

	propTypeErr  error     // Capture errors retrieving property types
	propTypeTime time.Time // Time when property types data was last updated
}

type Resource struct {
	Apigroup string
	Kind     string
}

var managedClusterResourceGvr = schema.GroupVersionResource{
	Group:    "cluster.open-cluster-management.io",
	Version:  "v1",
	Resource: "managedclusters",
}

func (shared *SharedData) getPropertyTypes(cache *Cache, ctx context.Context) (map[string]string, error) {

	// original query: select distinct key, jsonb_typeof(value) as datatype FROM search.resources,jsonb_each(data);
	var selectDs *goqu.SelectDataset
	//define schema:
	schemaTable := goqu.S("search").Table("resources")
	//data expression to get value and key
	jsb := goqu.L("jsonb_each(?)", goqu.C("data"))
	//select from these datasets
	ds := goqu.From(schemaTable, jsb)
	//select statement with orderby and distinct clause
	selectDs = ds.Select(goqu.L("key"), goqu.L("jsonb_typeof(?)",
		goqu.C("value")).As("datatype")).Distinct()

	query, params, sqlerr := selectDs.ToSQL()
	if sqlerr != nil {
		klog.Errorf("Error building Search query: %s", sqlerr.Error())
	}

	klog.V(5).Infof("Query for property datatypes: [%s] ", query)
	rows, err := cache.pool.Query(ctx, query, params...)
	if err != nil {
		klog.Errorf("Error resolving query [%s] with args [%+v]. Error: [%+v]", query, err)
		return nil, err
	}
	defer rows.Close()

	resourceTypeMap := make(map[string]string)
	for rows.Next() {
		var key, value string
		err = rows.Scan(&key, &value)

		if err != nil {
			klog.Errorf("Error %s retrieving rows for query:%s", err.Error(), query)
			continue
		}
		resourceTypeMap[key] = value

	}
	//cache query:
	shared.propTypes = resourceTypeMap
	shared.propTypeErr = err
	shared.propTypeTime = time.Now()

	return resourceTypeMap, err
}

func (cache *Cache) GetPropertyTypes(ctx context.Context) (map[string]string, error) {
	sharedData, err := cache.PopulateSharedCache(ctx)
	if err != nil {
		klog.Error("Error populating shared data cache: ", err)
		return nil, err
	}
	//store only the PropTypeCache to use in outside of rbac module
	propTypesMap := sharedData.propTypes

	return propTypesMap, nil
}

func (cache *Cache) PopulateSharedCache(ctx context.Context) (*SharedData, error) {

	if sharedCacheValid(&cache.shared) { //if all cache is valid we use cache data
		klog.V(5).Info("Using shared data from cache.")
		return &cache.shared, nil
	} else { //get data and cache

		var error error

		_, err := cache.shared.getPropertyTypes(cache, ctx)
		if err != nil {
			klog.Errorf("Error retrieving property resources. Error: [%+v]", err)
		} else {
			klog.V(6).Info("Successfully retrieved property resources!")
		}
		// get all cluster-scoped resources and cache in shared.csResources
		err = cache.shared.GetClusterScopedResources(cache, ctx)
		if err != nil {
			klog.Errorf("Error retrieving cluster scoped resources. Error: [%+v]", err)
		} else {
			klog.V(6).Info("Successfully retrieved cluster scoped resources!")
		}
		// get all namespaces in cluster and cache in shared.namespaces.
		err = cache.shared.GetSharedNamespaces(cache, ctx)
		if err != nil {
			klog.Errorf("Error retrieving shared namespaces. Error: [%+v]", err)
		} else {
			klog.V(6).Info("Successfully retrieved shared namespaces!")
		}
		// get all managed clustsers in cache
		err = cache.shared.GetManagedClusters(cache, ctx)
		if err == nil {
			klog.Errorf("Error retrieving managed clusters. Error: [%+v]", err)
		} else {
			klog.V(6).Info("Successfully retrieved managed clusters!")
		}

		return &cache.shared, error

	}

}

func (cache *Cache) sharedCacheDisabledClustersValid() bool {
	return cache.shared.dcErr == nil && time.Now().Before(
		cache.shared.dcUpdatedAt.Add(time.Duration(config.Cfg.SharedCacheTTL)*time.Millisecond))
}

func sharedCacheValid(shared *SharedData) bool {

	if (time.Now().Before(shared.csUpdatedAt.Add(time.Duration(config.Cfg.SharedCacheTTL) * time.Millisecond))) &&
		(time.Now().Before(shared.nsUpdatedAt.Add(time.Duration(config.Cfg.SharedCacheTTL) * time.Millisecond))) &&
		(time.Now().Before(shared.mcUpdatedAt.Add(time.Duration(config.Cfg.SharedCacheTTL) * time.Millisecond))) &&
		(time.Now().Before(shared.propTypeTime.Add(time.Duration(config.Cfg.SharedCacheTTL) * time.Millisecond))) {

		return true
	}
	return false
}

// Obtain all the cluster-scoped resources in the hub cluster that support list and watch.
// Equivalent to: `oc api-resources -o wide | grep false | grep watch | grep list`
func (shared *SharedData) GetClusterScopedResources(cache *Cache, ctx context.Context) error {

	// lock to prevent checking more than one at a time and check if cluster scoped resources already in cache
	shared.csLock.Lock()
	defer shared.csLock.Unlock()
	//clear previous cache
	shared.csResourcesMap = make(map[Resource]struct{})
	shared.csErr = nil
	klog.V(6).Info("Querying database for cluster-scoped resources.")

	// Building query to get cluster scoped resources
	// Original query: "SELECT DISTINCT(data->>apigroup, data->>kind) FROM search.resources WHERE
	// cluster='local-cluster' AND namespace=NULL"
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)
	query, _, err := ds.SelectDistinct(goqu.COALESCE(goqu.L(`"data"->>'apigroup'`), "").As("apigroup"),
		goqu.COALESCE(goqu.L(`"data"->>'kind_plural'`), "").As("kind")).
		Where(goqu.L(`"data"->>'_hubClusterResource'='true'`), goqu.L(`"data"->>'namespace'`).IsNull()).ToSQL()
	if err != nil {
		klog.Errorf("Error creating query [%s]. Error: [%+v]", query, err)
		shared.csErr = err
		shared.csResourcesMap = map[Resource]struct{}{}
		return shared.csErr
	}

	// var queryerr error
	rows, err := cache.pool.Query(ctx, query)
	if err != nil {
		klog.Errorf("Error resolving query [%s]. Error: [%+v]", query, err.Error())
		shared.csErr = err
		shared.csResourcesMap = map[Resource]struct{}{}

		return shared.csErr
	}

	if rows != nil {
		defer rows.Close()

		for rows.Next() {
			var kind, apigroup string
			err := rows.Scan(&apigroup, &kind)
			if err != nil {
				klog.Warning("Error %s retrieving rows for query:%s for apigroup %s and kind %s", err.Error(), query,
					apigroup, kind)
				continue
			}
			shared.csResourcesMap[Resource{Apigroup: apigroup, Kind: kind}] = struct{}{}
		}
	}
	shared.csUpdatedAt = time.Now()

	return shared.csErr
}

// Obtain all the namespaces in the hub cluster.
// Equivalent to `oc get namespaces`
func (shared *SharedData) GetSharedNamespaces(cache *Cache, ctx context.Context) error {
	shared.nsLock.Lock()
	defer shared.nsLock.Unlock()
	//empty previous cache
	shared.namespaces = nil
	shared.nsErr = nil

	klog.V(5).Info("Getting namespaces from Kube Client.")

	namespaceList, nsErr := cache.corev1Client.Namespaces().List(ctx, metav1.ListOptions{})
	if nsErr != nil {
		klog.Warning("Error resolving namespaces from KubeClient: ", nsErr)
		shared.nsErr = nsErr
		shared.nsUpdatedAt = time.Now()
		return shared.nsErr
	}

	// add namespaces to allNamespace List
	for _, n := range namespaceList.Items {
		shared.namespaces = append(shared.namespaces, n.Name)
	}
	shared.nsUpdatedAt = time.Now()

	return shared.nsErr
}

func (shared *SharedData) GetManagedClusters(cache *Cache, ctx context.Context) error {

	shared.mcLock.Lock()
	defer shared.mcLock.Unlock()
	// clear previous cache
	shared.managedClusters = nil
	shared.mcErr = nil

	managedClusters := make(map[string]struct{})

	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(managedClusterResourceGvr.GroupVersion())

	resourceObj, err := cache.dynamicClient.Resource(managedClusterResourceGvr).List(ctx, metav1.ListOptions{})

	if err != nil {
		klog.Warning("Error resolving ManagedClusters with dynamic client", err.Error())
		shared.mcErr = err
		shared.mcUpdatedAt = time.Now()
		return shared.mcErr
	}

	for _, item := range resourceObj.Items {
		if item.GetName() != "local-cluster" {
			managedClusters[item.GetName()] = struct{}{}
		}
	}

	shared.managedClusters = managedClusters
	shared.mcUpdatedAt = time.Now()
	return shared.mcErr

}

// Returns a map of managed clusters for which the search add-on has been disabled.
func (cache *Cache) GetDisabledClusters(ctx context.Context) (*map[string]struct{}, error) {
	uid, _ := cache.GetUserUID(ctx)
	userData, userDataErr := cache.GetUserData(ctx)
	if userDataErr != nil {
		return nil, userDataErr
	}
	// lock to prevent the query from running repeatedly
	cache.shared.dcLock.Lock()
	defer cache.shared.dcLock.Unlock()

	if !cache.sharedCacheDisabledClustersValid() {
		klog.V(5).Info("DisabledClusters cache empty or expired. Querying database.")
		// - running query to get search addon disabled clusters")
		//run query and get disabled clusters
		if disabledClustersFromQuery, err := cache.findSrchAddonDisabledClusters(ctx); err != nil {
			klog.Error("Error retrieving Search Addon disabled clusters: ", err)
			cache.setDisabledClusters(map[string]struct{}{}, err)
			return nil, err
		} else {
			cache.setDisabledClusters(*disabledClustersFromQuery, nil)
		}
	}

	//check if user has access to disabled clusters
	userAccessClusters := disabledClustersForUser(cache.shared.disabledClusters, userData.ManagedClusters, uid)
	if len(userAccessClusters) > 0 {
		klog.V(5).Info("user ", uid, " has access to Search Addon disabled clusters ")
		return &userAccessClusters, cache.shared.dcErr

	} else {
		klog.V(5).Info("user does not have access to Search Addon disabled clusters ")
		return &map[string]struct{}{}, nil
	}

}

func disabledClustersForUser(disabledClusters map[string]struct{},
	userClusters map[string]struct{}, uid string) map[string]struct{} {
	userAccessDisabledClusters := map[string]struct{}{}
	for disabledCluster := range disabledClusters {
		if _, userHasAccess := userClusters[disabledCluster]; userHasAccess { //user has access
			klog.V(7).Info("user ", uid, " has access to search addon disabled cluster: ", disabledCluster)
			userAccessDisabledClusters[disabledCluster] = struct{}{}
		}
	}
	return userAccessDisabledClusters
}

func (cache *Cache) setDisabledClusters(disabledClusters map[string]struct{}, err error) {
	cache.shared.disabledClusters = disabledClusters
	cache.shared.dcUpdatedAt = time.Now()
	cache.shared.dcErr = err
}

// Build the query to find any ManagedClusters where the search addon is disabled.
func buildSearchAddonDisabledQuery(ctx context.Context) (string, error) {
	var selectDs *goqu.SelectDataset

	//FROM CLAUSE
	schemaTable1 := goqu.S("search").Table("resources").As("mcInfo")
	schemaTable2 := goqu.S("search").Table("resources").As("srchAddon")

	// For each ManagedClusterInfo resource in the hub,
	// we should have a matching ManagedClusterAddOn
	// with name=search-collector in the same namespace.
	ds := goqu.From(schemaTable1).
		LeftOuterJoin(schemaTable2,
			goqu.On(goqu.L(`"mcInfo".data->>?`, "name").Eq(goqu.L(`"srchAddon".data->>?`, "namespace")),
				goqu.L(`"srchAddon".data->>?`, "kind").Eq("ManagedClusterAddOn"),
				goqu.L(`"srchAddon".data->>?`, "name").Eq("search-collector")))

	//SELECT CLAUSE
	selectDs = ds.SelectDistinct(goqu.L(`"mcInfo".data->>?`, "name").As("srchAddonDisabledCluster"))

	// WHERE CLAUSE
	var whereDs []exp.Expression

	// select ManagedClusterInfo
	whereDs = append(whereDs, goqu.L(`"mcInfo".data->>?`, "kind").Eq("ManagedClusterInfo"))
	// addon uid will be null if addon is disabled
	whereDs = append(whereDs, goqu.L(`"srchAddon".uid`).IsNull())
	// exclude local-cluster
	whereDs = append(whereDs, goqu.L(`"mcInfo".data->>?`, "name").Neq("local-cluster"))

	//Get the query
	sql, params, err := selectDs.Where(whereDs...).ToSQL()
	if err != nil {
		klog.Errorf("Error building Query for managed clusters with Search addon disabled: %s", err.Error())
		return "", err
	}
	klog.V(3).Infof("Query for managed clusters with Search addon disabled: %s %s\n", sql, params)
	return sql, nil
}

func (cache *Cache) findSrchAddonDisabledClusters(ctx context.Context) (*map[string]struct{}, error) {
	disabledClusters := make(map[string]struct{})
	// build the query
	sql, queryBuildErr := buildSearchAddonDisabledQuery(ctx)
	if queryBuildErr != nil {
		klog.Error("Error fetching SearchAddon disabled cluster results from db ", queryBuildErr)
		cache.setDisabledClusters(disabledClusters, queryBuildErr)
		return &disabledClusters, queryBuildErr
	}
	// run the query
	rows, err := cache.pool.Query(ctx, sql)
	if err != nil {
		klog.Error("Error fetching SearchAddon disabled cluster results from db ", err)
		cache.setDisabledClusters(disabledClusters, err)
		return &disabledClusters, err
	}

	if rows != nil {
		for rows.Next() {
			var srchAddonDisabledCluster string
			err := rows.Scan(&srchAddonDisabledCluster)
			if err != nil {
				klog.Errorf("Error %s resolving addon disabled cluster name for query: %s", err.Error())
				continue // skip and continue in case of scan error
			}
			disabledClusters[srchAddonDisabledCluster] = struct{}{}
		}
		//Since cache was not valid, update shared cache with disabled clusters result
		cache.setDisabledClusters(disabledClusters, nil)
		defer rows.Close()
	}
	return &disabledClusters, err
}
