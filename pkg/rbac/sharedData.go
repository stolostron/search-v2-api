// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"sync"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/driftprogramming/pgxpoolmock"
	"github.com/stolostron/search-v2-api/pkg/metrics"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

// Cache data shared across all users.
type SharedData struct {
	// These are the data fields.
	csResourcesMap   map[Resource]struct{}
	disabledClusters map[string]struct{}
	managedClusters  map[string]struct{}
	namespaces       []string
	propTypes        map[string]string

	// Metadata to manage the state of the cached data.
	csrCache cacheMetadata // csResourcesMap
	dcCache  cacheMetadata // disabledClusters
	mcCache  cacheMetadata // managedClusters
	nsCache  cacheMetadata // namespaces
	ptCache  cacheMetadata // propTypes

	// Clients to external APIs to be replaced with a mock by unit tests.
	dynamicClient dynamic.Interface
	pool          pgxpoolmock.PgxPool // Database client
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
var namespacesGvr = schema.GroupVersionResource{
	Group:    "",
	Version:  "v1",
	Resource: "namespaces",
}

// Query the database to get all properties and their types.
// Sample query:
//
//	select distinct key, jsonb_typeof(value) as datatype FROM search.resources,jsonb_each(data);
func (shared *SharedData) getPropertyTypes(ctx context.Context) (map[string]string, error) {
	propTypeMap := make(map[string]string)
	var selectDs *goqu.SelectDataset

	// define schema
	schemaTable := goqu.S("search").Table("resources")

	// data expression to get value and key
	jsb := goqu.L("jsonb_each(?)", goqu.C("data"))

	// jsonb_type of returns: object, array, string, number, boolean, and null. Enhance by trying to add 'timestamp' type when strings match ISO 8601 timestamp format
	caseExpr := goqu.Case().
		When(
			goqu.And(
				goqu.L("jsonb_typeof(value) = 'string'"),
				// matches "2025-10-13T15:42:00 -> permits milliseconds and different timezones at end
				goqu.L(`value::text ~ '^"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}'`),
			),
			"timestamp",
		).Else(goqu.L("jsonb_typeof(value)"))

	selectDs = goqu.From(schemaTable, jsb).
		Select(goqu.L("key"), caseExpr.As("datatype")).
		Distinct()

	query, params, err := selectDs.ToSQL()
	if err != nil {
		klog.Errorf("Error building Search query: %s", err.Error())
		return propTypeMap, err
	}

	klog.V(5).Infof("Query for property datatypes: [%s] ", query)
	rows, err := shared.pool.Query(ctx, query, params...)
	if err != nil {
		klog.Errorf("Error resolving property types query [%s] with args [%+v]. Error: [%+v]", query, params, err)
		return propTypeMap, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		err = rows.Scan(&key, &value)
		if err != nil {
			klog.Errorf("Error %s scanning value for getPropertyTypes:%s", err.Error(), query)
			continue
		}
		propTypeMap[key] = value

	}
	// NOTE: we will have to do this for any property that is not in the data field,
	// especially if new columns are added to the resources table.
	propTypeMap["cluster"] = "string"

	klog.Info("Successfully fetched property types from the database.")
	//cache results:
	shared.ptCache.lock.Lock()
	defer shared.ptCache.lock.Unlock()
	shared.propTypes = propTypeMap
	shared.ptCache.err = err

	return propTypeMap, err
}

// Get all available properties and their types. Will use cached data if available.
//
//	refresh - forces cached data to refresh from database.
func (cache *Cache) GetPropertyTypes(ctx context.Context, refresh bool) (map[string]string, error) {
	cache.shared.ptCache.lock.RLock()

	// check if propTypes data in cache and not nil and return
	if len(cache.shared.propTypes) > 0 && cache.shared.ptCache.err == nil && !refresh {
		// copy cache.shared.propTypes map to avoid sharing reference
		propTypesMap := make(map[string]string, len(cache.shared.propTypes))
		for k, v := range cache.shared.propTypes {
			propTypesMap[k] = v
		}
		defer cache.shared.ptCache.lock.RUnlock()
		return propTypesMap, nil

	} else {
		klog.V(6).Info("Getting property types from database.")
		// If we have to modify cache.shared.ptCache, we have to first release the read lock, we can't wait for the defer
		cache.shared.ptCache.lock.RUnlock()
		// run query to refresh data
		propTypes, err := cache.shared.getPropertyTypes(ctx)
		if err != nil {
			klog.Errorf("Error retrieving property types. Error: [%+v]", err)
			return map[string]string{}, err
		} else {
			// Record property type for managedHub - for Global Search - https://issues.redhat.com/browse/ACM-10019
			propTypes["managedHub"] = "string"
			klog.V(6).Info("Successfully retrieved property types!")

			return propTypes, nil
		}
	}
}

func (shared *SharedData) PopulateSharedCache(ctx context.Context) {
	defer metrics.SlowLog("PopulateSharedCache", 0)()
	if shared.isValid() { // if all cache is valid we use cache data
		klog.V(5).Info("Using shared data from cache.")
		return
	} else { // get data and add to cache
		var wg sync.WaitGroup

		// get hub cluster-scoped resources and cache in shared.csResources
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := shared.getClusterScopedResources(ctx)
			if err != nil {
				klog.Errorf("Error retrieving cluster scoped resources. Error: [%+v]", err)
			}
		}()

		// get hub cluster namespaces and cache in shared.namespaces.
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := shared.getNamespaces(ctx)
			if err != nil {
				klog.Errorf("Error retrieving shared namespaces. Error: [%+v]", err)
			}
		}()

		// get managed clusters and cache in shared.managedClusters
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := shared.getManagedClusters(ctx)
			if err != nil {
				klog.Errorf("Error retrieving managed clusters. Error: [%+v]", err)
			}
		}()

		wg.Wait() // Wait for async go routines to complete.

	}
}

func (shared *SharedData) isValid() bool {
	if shared.csrCache.isValid() && shared.nsCache.isValid() && shared.mcCache.isValid() {
		return true
	}
	return false
}

// Obtain all the cluster-scoped resources in the hub cluster that support list and watch
// Get the list of resources in the database where namespace field is null.
// Equivalent to: `oc api-resources -o wide | grep false | grep watch | grep list`
func (shared *SharedData) getClusterScopedResources(ctx context.Context) error {
	defer metrics.SlowLog("SharedData::getClusterScopedResources", 0)()
	// lock to prevent checking more than one at a time and check if cluster scoped resources already in cache
	shared.csrCache.lock.Lock()
	defer shared.csrCache.lock.Unlock()
	//clear previous cache
	shared.csResourcesMap = make(map[Resource]struct{})
	shared.csrCache.err = nil
	klog.V(6).Info("Querying database for cluster-scoped resources.")

	// Building query to get cluster scoped resources
	// Original query: "SELECT DISTINCT data->>'apigroup', data->>'kind_plural' FROM search.resources WHERE
	// data?'_hubClusterResource' AND data?'namespace' is FALSE"
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)
	query, _, err := ds.SelectDistinct(goqu.COALESCE(goqu.L(`"data"->>'apigroup'`), "").As("apigroup"),
		goqu.COALESCE(goqu.L(`"data"->>'kind_plural'`), "").As("kind")).
		Where(goqu.L("???", goqu.C("data"), goqu.Literal("?"), "_hubClusterResource"),
			goqu.L("???", goqu.C("data"), goqu.Literal("?"), "namespace").IsFalse()).ToSQL()
	if err != nil {
		klog.Errorf("Error creating query [%s]. Error: [%+v]", query, err)
		shared.csrCache.err = err
		shared.csResourcesMap = map[Resource]struct{}{}
		return shared.csrCache.err
	}

	rows, err := shared.pool.Query(ctx, query)
	if err != nil {
		klog.Errorf("Error resolving cluster scoped resources. Query [%s]. Error: [%+v]", query, err.Error())
		shared.csrCache.err = err
		shared.csResourcesMap = map[Resource]struct{}{}

		return shared.csrCache.err
	}

	if rows != nil {
		defer rows.Close()

		for rows.Next() {
			var kind, apigroup string
			err := rows.Scan(&apigroup, &kind)
			if err != nil {
				klog.Warningf("Error %s retrieving rows for query:%s for apigroup %s and kind %s", err.Error(), query,
					apigroup, kind)
				continue
			}
			shared.csResourcesMap[Resource{Apigroup: apigroup, Kind: kind}] = struct{}{}
		}
	}
	shared.csrCache.updatedAt = time.Now()

	return shared.csrCache.err
}

// Obtain all the namespaces in the hub cluster.
// Equivalent to `oc get namespaces`
func (shared *SharedData) getNamespaces(ctx context.Context) ([]string, error) {
	defer metrics.SlowLog("getSharedNamespaces", 100*time.Millisecond)()

	// Return cached namespaces if valid.
	shared.nsCache.lock.RLock()
	if shared.nsCache.isValid() {
		namespaces := shared.namespaces
		shared.nsCache.lock.RUnlock()
		return namespaces, nil
	}
	shared.nsCache.lock.RUnlock()

	klog.V(5).Info("Getting namespaces from Kube Client.")
	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(namespacesGvr.GroupVersion())

	namespaceList, nsErr := shared.dynamicClient.Resource(namespacesGvr).List(ctx, metav1.ListOptions{})

	if nsErr != nil {
		shared.nsCache.lock.Lock()
		defer shared.nsCache.lock.Unlock()
		klog.Warning("Error resolving namespaces from KubeClient: ", nsErr)
		shared.nsCache.err = nsErr
		shared.nsCache.updatedAt = time.Now()
		return nil, shared.nsCache.err
	}

	namespaces := make([]string, 0, len(namespaceList.Items))
	for _, n := range namespaceList.Items {
		namespaces = append(namespaces, n.GetName())
	}

	shared.nsCache.lock.Lock()
	defer shared.nsCache.lock.Unlock()
	shared.nsCache.err = nil
	shared.namespaces = namespaces
	shared.nsCache.updatedAt = time.Now()

	return shared.namespaces, shared.nsCache.err
}

// Obtain all the managedclusters.
// Equivalent to `oc get managedclusters`
func (shared *SharedData) getManagedClusters(ctx context.Context) error {
	defer metrics.SlowLog("getManagedClusters", 100*time.Millisecond)()

	shared.mcCache.lock.Lock()
	defer shared.mcCache.lock.Unlock()
	// clear previous cache
	shared.managedClusters = nil
	shared.mcCache.err = nil

	managedClusters := make(map[string]struct{})

	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(managedClusterResourceGvr.GroupVersion())

	resourceObj, err := shared.dynamicClient.Resource(managedClusterResourceGvr).List(ctx, metav1.ListOptions{})

	if err != nil {
		klog.Warning("Error resolving ManagedClusters with dynamic client", err.Error())
		shared.mcCache.err = err
		shared.mcCache.updatedAt = time.Now()
		return shared.mcCache.err
	}

	for _, item := range resourceObj.Items {
		// Add to list if it is not local-cluster
		if _, ok := item.GetLabels()["local-cluster"]; !ok {
			managedClusters[item.GetName()] = struct{}{}
		}
	}

	klog.V(3).Info("List of managed clusters in shared data: ", managedClusters)
	shared.managedClusters = managedClusters
	shared.mcCache.updatedAt = time.Now()
	return shared.mcCache.err

}

// Returns a map of managed clusters for which the search add-on has been disabled.
func (cache *Cache) GetDisabledClusters(ctx context.Context) (*map[string]struct{}, error) {
	uid, _ := cache.GetUserUID(ctx)
	userData, userDataErr := cache.GetUserData(ctx)
	if userDataErr != nil {
		return nil, userDataErr
	}
	// lock to prevent the query from running repeatedly
	cache.shared.dcCache.lock.Lock()
	defer cache.shared.dcCache.lock.Unlock()

	if !cache.shared.dcCache.isValid() {
		klog.V(5).Info("DisabledClusters cache empty or expired. Querying database.")
		// - running query to get search addon disabled clusters")
		//run query and get disabled clusters
		if disabledClustersFromQuery, err := cache.shared.findSrchAddonDisabledClusters(ctx); err != nil {
			klog.Error("Error retrieving Search Addon disabled clusters: ", err)
			cache.shared.setDisabledClusters(map[string]struct{}{}, err)
			return nil, err
		} else {
			cache.shared.setDisabledClusters(*disabledClustersFromQuery, nil)
		}
	}

	//check if user has access to disabled clusters
	userAccessClusters := disabledClustersForUser(cache.shared.disabledClusters, userData.ManagedClusters, uid)
	if len(userAccessClusters) > 0 {
		klog.V(5).Info("user ", uid, " has access to Search Addon disabled clusters ")
		return &userAccessClusters, cache.shared.dcCache.err

	} else {
		klog.V(5).Info("user does not have access to Search Addon disabled clusters ")
		return &map[string]struct{}{}, nil
	}

}

// Filters the list of disabled clusters to only include the clusters the user has access.
func disabledClustersForUser(disabledClusters map[string]struct{},
	userClusters map[string]struct{}, uid string) map[string]struct{} {

	// If user has access to all clusters, return the full list of disabled clusters.
	if _, exists := userClusters["*"]; exists {
		klog.V(7).Infof("user %s has access to all clusters with search add-on disabled", uid)
		return disabledClusters
	}

	// Filter out the clusters the user has access to.
	userAccessDisabledClusters := map[string]struct{}{}
	for disabledCluster := range disabledClusters {
		if _, userHasAccess := userClusters[disabledCluster]; userHasAccess { // user has access to cluster.
			userAccessDisabledClusters[disabledCluster] = struct{}{}
		}
	}
	klog.V(7).Infof("user %s has access these clusters with search add-on disabled: %+v", uid, userAccessDisabledClusters)
	return userAccessDisabledClusters
}

func (shared *SharedData) setDisabledClusters(disabledClusters map[string]struct{}, err error) {
	shared.disabledClusters = disabledClusters
	shared.dcCache.updatedAt = time.Now()
	shared.dcCache.err = err
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

func (shared *SharedData) findSrchAddonDisabledClusters(ctx context.Context) (*map[string]struct{}, error) {
	disabledClusters := make(map[string]struct{})
	// build the query
	sql, queryBuildErr := buildSearchAddonDisabledQuery(ctx)
	if queryBuildErr != nil {
		klog.Error("Error fetching SearchAddon disabled cluster results from db ", queryBuildErr)
		shared.setDisabledClusters(disabledClusters, queryBuildErr)
		return &disabledClusters, queryBuildErr
	}
	// run the query
	rows, err := shared.pool.Query(ctx, sql)
	if err != nil {
		klog.Error("Error fetching SearchAddon disabled cluster results from db ", err)
		shared.setDisabledClusters(disabledClusters, err)
		return &disabledClusters, err
	}

	if rows != nil {
		for rows.Next() {
			var srchAddonDisabledCluster string
			err := rows.Scan(&srchAddonDisabledCluster)
			if err != nil {
				klog.Errorf("Error %s resolving addon disabled cluster name for query: %s", err.Error(), sql)
				continue // skip and continue in case of scan error
			}
			disabledClusters[srchAddonDisabledCluster] = struct{}{}
		}
		//Since cache was not valid, update shared cache with disabled clusters result
		shared.setDisabledClusters(disabledClusters, nil)
		defer rows.Close()
	}
	return &disabledClusters, err
}

// SSRR has resources that are clusterscoped too - check if a resource is clusterscoped
func (shared *SharedData) isClusterScoped(kindPlural, apigroup string) bool {
	// lock to prevent checking more than one at a time
	shared.csrCache.lock.Lock()
	defer shared.csrCache.lock.Unlock()
	var ok bool
	resource := Resource{Apigroup: apigroup, Kind: kindPlural}
	_, ok = shared.csResourcesMap[resource]
	if ok {
		klog.V(9).Infof("resource is ClusterScoped %+v", resource)
	} else {
		consoleApiGrp := "console.openshift.io"
		// check if it is a cluster-scoped resource in this list
		openshiftClusterScopedRes := map[Resource]struct{}{
			{Apigroup: "authorization.openshift.io", Kind: "clusterroles"}:  {},
			{Apigroup: "", Kind: "clusterroles"}:                            {},
			{Apigroup: consoleApiGrp, Kind: "consoleexternalloglinks"}:      {},
			{Apigroup: consoleApiGrp, Kind: "consolelinks"}:                 {},
			{Apigroup: consoleApiGrp, Kind: "consolenotifications"}:         {},
			{Apigroup: consoleApiGrp, Kind: "consoleyamlsamples"}:           {},
			{Apigroup: "project.openshift.io", Kind: "projects"}:            {},
			{Apigroup: "", Kind: "projects"}:                                {},
			{Apigroup: "project.openshift.io", Kind: "projectrequests"}:     {},
			{Apigroup: "", Kind: "projectrequests"}:                         {},
			{Apigroup: "oauth.openshift.io", Kind: "useroauthaccesstokens"}: {},
		}
		if _, ok = openshiftClusterScopedRes[resource]; ok {
			klog.V(9).Infof("resource is openshiftClusterScoped %+v", resource)
		}
	}
	return ok
}
