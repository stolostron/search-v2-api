package rbac

import (
	"context"
	"fmt"
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

	//lock to prevent checking more than one at a time
	shared.csLock.Lock()
	defer shared.csLock.Unlock()
	if shared.csResources != nil &&
		time.Now().Before(shared.csUpdatedAt.Add(time.Duration(config.Cfg.SharedCacheTTL)*time.Millisecond)) {
		klog.V(8).Info("Using cluster scoped resources from cache.")
		return shared.csResources, shared.csErr
	}
	klog.V(6).Info("Querying database for cluster-scoped resources.")
	shared.csErr = nil // Clear previous errors.

	// if data not in cache or expired
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)

	//"SELECT DISTINCT(data->>apigroup, data->>kind) FROM search.resources WHERE
	// cluster='local-cluster' AND namespace=NULL"
	query, _, err := ds.SelectDistinct(goqu.COALESCE(goqu.L(`"data"->>'apigroup'`), "").As("apigroup"),
		goqu.COALESCE(goqu.L(`"data"->>'kind'`), "").As("kind")).
		Where(goqu.L(`"cluster"::TEXT = 'local-cluster'`), goqu.L(`"data"->>'namespace'`).IsNull()).ToSQL()
	if err != nil {
		klog.Errorf("Error creating query [%s]. Error: [%+v]", query, err)
		shared.csErr = err
	}

	rows, queryerr := cache.pool.Query(ctx, query)
	if queryerr != nil {
		klog.Errorf("Error resolving query [%s]. Error: [%+v]", query, queryerr.Error())
		shared.csErr = queryerr
		shared.csResources = []resource{}
		return shared.csResources, shared.csErr
	}

	// var resource *resource
	if rows != nil {
		defer rows.Close()

		for rows.Next() {
			var kind string
			var apigroup string
			err := rows.Scan(&apigroup, &kind)
			if err != nil {
				klog.Errorf("Error %s retrieving rows for query:%s for apigroup %s and kind %s", err.Error(), query,
					apigroup, kind)
				shared.csErr = err
				continue
			}

			shared.csResources = append(shared.csResources, resource{apigroup: apigroup, kind: kind})

		}
	}

	// //gather all namespaces in the cluster and cache in shared namespaces cache
	// var allNamespaces []string
	// if len(cache.shared.namespaces) > 0 {
	// 	klog.V(5).Info("Using namespaces from shared cache")
	// 	allNamespaces = append(allNamespaces, cache.shared.namespaces...)

	// } else {

	// 	klog.V(5).Info("Getting namespaces from Kube Client..")

	// 	cache.restConfig = config.GetClientConfig()
	// 	clientset, err := kubernetes.NewForConfig(cache.restConfig)
	// 	if err != nil {
	// 		klog.Warning("Error with creating a new clientset.", err.Error())
	// 	}
	// 	namespaceList, kubeErr := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	// 	if kubeErr != nil {
	// 		klog.Warning("Error resolving namespaces from KubeClient: ", kubeErr)
	// 		shared.nsErr = kubeErr
	// 		shared.namespaces = []string{}
	// 		return shared.csResources, kubeErr
	// 	}

	// 	// add namespaces to allNamespace List
	// 	for _, n := range namespaceList.Items {
	// 		allNamespaces = append(allNamespaces, n.Name)
	// 	}
	// }
	shared.nsLock.Lock()
	defer shared.nsLock.Unlock()
	allNamespaces, nsErr := shared.GetSharedNamespaces(cache, ctx)
	if nsErr != nil {
		shared.nsErr = nsErr
	}

	// Then update the cache.
	shared.namespaces = allNamespaces
	shared.nsUpdatedAt = time.Now()

	shared.csUpdatedAt = time.Now()
	fmt.Println(shared.csResources)

	return shared.csResources, shared.csErr
}

//gather all namespaces in the cluster and cache in shared namespaces cache
func (shared *SharedData) GetSharedNamespaces(cache *Cache, ctx context.Context) ([]string, error) {
	var allNamespaces []string
	if len(cache.shared.namespaces) > 0 {
		klog.V(5).Info("Using namespaces from shared cache")
		allNamespaces = append(allNamespaces, cache.shared.namespaces...)

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
			shared.namespaces = append(shared.namespaces, allNamespaces...)
			return shared.namespaces, shared.nsErr
		}

		// add namespaces to allNamespace List
		for _, n := range namespaceList.Items {
			allNamespaces = append(allNamespaces, n.Name)
		}

	}
	return allNamespaces, shared.nsErr
}
