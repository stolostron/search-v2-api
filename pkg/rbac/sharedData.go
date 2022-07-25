package rbac

import (
	"context"
	"sync"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/stolostron/search-v2-api/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// Cache data shared across all users.
type SharedData struct {
	// These are the data fields.
	csResources []resource // Cluster-scoped resources (ie. Node, ManagedCluster)
	namespaces  []string

	// These are internal objects to track the state of the cache.
	csErr       error      // Capture errors retrieving cluster-scoped resources.
	csLock      sync.Mutex // Locks the csResources map while updating it.
	csUpdatedAt time.Time  // Time when cluster-scoped data was last updated.
	nsErr       error      // Capture errors retrieving namespaces.
	nsLock      sync.Mutex // Locks the namespaces array while updating it.
	nsUpdatedAt time.Time  // Time when namespaces data was last updated.
}

type resource struct {
	apigroup string
	kind     string
}

func (cache *Cache) ClusterScopedResources(ctx context.Context) ([]resource, error) {
	clusterScoped, err := cache.shared.getClusterScopedResources(cache, ctx)
	return clusterScoped, err
}

func (shared *SharedData) getClusterScopedResources(cache *Cache, ctx context.Context) ([]resource,
	error) {

	// lock to prevent checking more than one at a time and check if cluster scoped resources already in cache
	shared.csLock.Lock()
	defer shared.csLock.Unlock()
	if shared.csResources == nil ||
		time.Now().After(shared.csUpdatedAt.Add(time.Duration(config.Cfg.SharedCacheTTL)*time.Millisecond)) {

		// if not in cache query database
		klog.V(6).Info("Querying database for cluster-scoped resources.")
		shared.csErr = nil // Clear previous errors.

		// Building query to get cluster scoped resources
		// Original query: "SELECT DISTINCT(data->>apigroup, data->>kind) FROM search.resources WHERE
		// cluster='local-cluster' AND namespace=NULL"

		schemaTable := goqu.S("search").Table("resources")
		ds := goqu.From(schemaTable)
		query, _, err := ds.SelectDistinct(goqu.COALESCE(goqu.L(`"data"->>'apigroup'`), "").As("apigroup"),
			goqu.COALESCE(goqu.L(`"data"->>'kind_plural'`), "").As("kind")).
			Where(goqu.L(`"cluster"::TEXT = 'local-cluster'`), goqu.L(`"data"->>'namespace'`).IsNull()).ToSQL()
		if err != nil {
			klog.Errorf("Error creating query [%s]. Error: [%+v]", query, err)
			shared.csErr = err
			shared.csResources = []resource{}
			return shared.csResources, shared.csErr
		}

		rows, queryerr := cache.pool.Query(ctx, query)
		if queryerr != nil {
			klog.Errorf("Error resolving query [%s]. Error: [%+v]", query, queryerr.Error())
			shared.csErr = queryerr
			shared.csResources = []resource{}
			return shared.csResources, shared.csErr
		}

		if rows != nil {
			defer rows.Close()

			for rows.Next() {
				var kind string
				var apigroup string
				err := rows.Scan(&apigroup, &kind)
				if err != nil {
					klog.Errorf("Error %s retrieving rows for query:%s for apigroup %s and kind %s", err.Error(), query,
						apigroup, kind)
					continue
				}

				shared.csResources = append(shared.csResources, resource{apigroup: apigroup, kind: kind})

			}
		}

		// get all namespaces in cluster and cache in shared.namespaces.
		shared.nsLock.Lock()
		defer shared.nsLock.Unlock()
		allNamespaces, nsErr := shared.GetSharedNamespaces(cache, ctx)
		if nsErr != nil {
			shared.nsErr = nsErr
		}

		// update the cache.
		shared.namespaces = allNamespaces
		shared.csUpdatedAt = time.Now()
	} else {
		klog.V(8).Info("Using cluster scoped resources from cache.")
	}

	return shared.csResources, shared.csErr
}

func (shared *SharedData) GetSharedNamespaces(cache *Cache, ctx context.Context) ([]string, error) {

	if len(shared.namespaces) > 0 &&
		time.Now().Before(shared.nsUpdatedAt.Add(time.Duration(config.Cfg.SharedCacheTTL)*time.Millisecond)) {
		klog.V(5).Info("Using namespaces from shared cache")

	} else {

		klog.V(5).Info("Getting namespaces from Kube Client..")

		cache.restConfig = config.GetClientConfig()
		clientset, err := kubernetes.NewForConfig(cache.restConfig)
		if err != nil {
			klog.Warning("Error with creating a new clientset.", err.Error())

		}

		namespaceList, kubeErr := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})

		if kubeErr != nil {
			klog.Warning("Error resolving namespaces from KubeClient: ", kubeErr)
			shared.nsErr = kubeErr
			return shared.namespaces, shared.nsErr
		}

		// add namespaces to allNamespace List
		for _, n := range namespaceList.Items {
			shared.namespaces = append(shared.namespaces, n.Name)
		}
		shared.nsUpdatedAt = time.Now()

	}
	shared.nsErr = nil
	return shared.namespaces, shared.nsErr
}
