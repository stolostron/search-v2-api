package rbac

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/stolostron/search-v2-api/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

// Cache data shared across all users.
type SharedData struct {
	// These are the data fields.
	csResources      []Resource // Cluster-scoped resources (ie. Node, ManagedCluster)
	csResourcesMap   map[Resource]struct{}
	namespaces       []string
	managedClusters  []string
	disabledClusters map[string]struct{}

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

func (cache *Cache) PopulateSharedCache(ctx context.Context) error {

	if sharedCacheValid(&cache.shared) { //if all cache is valid we use cache data
		klog.V(5).Info("Using shared data from cache.")
		return nil
	} else { //get data and cache

		var error error
		// get all cluster-scoped resources and cache in shared.csResources
		err := cache.shared.GetClusterScopedResources(cache, ctx)
		if err == nil {
			klog.V(6).Info("Successfully retrieved cluster scoped resources!")
		} else {
			error = err
		}
		// get all namespaces in cluster and cache in shared.namespaces.
		err = cache.shared.GetSharedNamespaces(cache, ctx)
		if err == nil {
			klog.V(6).Info("Successfully retrieved shared namespaces!")
		} else {
			error = err
		}
		// get all managed clustsers in cache
		err = cache.shared.GetManagedClusters(cache, ctx)
		if err == nil {
			klog.V(6).Info("Successfully retrieved managed clusters!")
		} else {
			error = err
		}
		return error

	}

}

func (cache *Cache) SharedCacheDisabledClustersValid() bool {
	return cache.shared.dcErr == nil && time.Now().Before(cache.shared.dcUpdatedAt.Add(time.Duration(config.Cfg.SharedCacheTTL)*time.Millisecond))
}

func sharedCacheValid(shared *SharedData) bool {

	if (time.Now().Before(shared.csUpdatedAt.Add(time.Duration(config.Cfg.SharedCacheTTL) * time.Millisecond))) &&
		(time.Now().Before(shared.nsUpdatedAt.Add(time.Duration(config.Cfg.SharedCacheTTL) * time.Millisecond))) &&
		(time.Now().Before(shared.mcUpdatedAt.Add(time.Duration(config.Cfg.SharedCacheTTL) * time.Millisecond))) {

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
	shared.csResources = make([]Resource, 0)
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
		shared.csResources = []Resource{}
		return shared.csErr
	}

	rows, queryerr := cache.pool.Query(ctx, query)
	if queryerr != nil {
		klog.Errorf("Error resolving query [%s]. Error: [%+v]", query, queryerr.Error())
		shared.csErr = queryerr
		shared.csResources = []Resource{}
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
			shared.csResources = append(shared.csResources, Resource{Apigroup: apigroup, Kind: kind})

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

	var managedClusters []string

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
			managedClusters = append(managedClusters, item.GetName())
		}
	}

	shared.managedClusters = managedClusters
	shared.mcUpdatedAt = time.Now()
	return shared.mcErr

}

func (cache *Cache) GetDisabledClusters() (*map[string]struct{}, error) {

	if cache.SharedCacheDisabledClustersValid() {
		klog.V(5).Info("Search Addon DisabledClusters Cache valid")
	} else {
		klog.V(5).Info("DisabledClusters Cache not valid") // - running query to get search addon disabled clusters")
		cache.SetDisabledClusters(map[string]struct{}{}, fmt.Errorf("addon disabled clusters cache is invalid"))
	}
	cache.shared.dcLock.Lock()
	defer cache.shared.dcLock.Unlock()
	return &cache.shared.disabledClusters, cache.shared.dcErr
}

func (cache *Cache) SetDisabledClusters(disabledClusters map[string]struct{}, err error) {
	cache.shared.dcLock.Lock()
	defer cache.shared.dcLock.Unlock()
	cache.shared.disabledClusters = disabledClusters
	cache.shared.dcUpdatedAt = time.Now()
	cache.shared.dcErr = err
}
