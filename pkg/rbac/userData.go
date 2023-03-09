// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/metric"
	authv1 "k8s.io/api/authentication/v1"
	authz "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

const impersonationConfigCreationerror = "Error creating clientset with impersonation config"

// Contains data about the resources the user is allowed to access.
type UserDataCache struct {
	UserData
	userInfo authv1.UserInfo

	// Metadata to manage the state of the cached data.
	clustersCache cacheMetadata
	csrCache      cacheMetadata
	nsrCache      cacheMetadata

	// Client to external API to be replaced with a mock by unit tests.
	authzClient v1.AuthorizationV1Interface
}

// Stuct to keep a copy of users access
type UserData struct {
	CsResources     []Resource            // Cluster-scoped resources on hub the user has list access.
	NsResources     map[string][]Resource // Namespaced resources on hub the user has list access.
	ManagedClusters map[string]struct{}   // Managed clusters where the user has view access.
}

// Get user's UID
// Note: kubeadmin gets an empty string for uid
func (cache *Cache) GetUserUID(ctx context.Context) (string, authv1.UserInfo) {
	authKey := ctx.Value(ContextAuthTokenKey)
	if authKey != nil {
		clientToken := authKey.(string)

		//get uid from tokenreview
		if tokenReview, err := cache.GetTokenReview(ctx, clientToken); err == nil {
			uid := tokenReview.Status.User.UID
			klog.V(9).Info("Found uid: ", uid, " for user: ", tokenReview.Status.User.Username)
			return uid, tokenReview.Status.User
		} else {
			klog.Error("Error finding uid for user: ", tokenReview.Status.User.Username, err)
			return "noUidFound", authv1.UserInfo{}
		}
	} else {
		klog.Error("Error finding uid for user: ContextAuthTokenKey IS NOT SET ")
		return "noUidFound", authv1.UserInfo{}
	}
}

func (cache *Cache) GetUserDataCache(ctx context.Context,
	authzClient v1.AuthorizationV1Interface) (*UserDataCache, error) {

	var user *UserDataCache
	var uid string
	var err error
	var userInfo authv1.UserInfo
	// get uid from tokenreview
	if uid, userInfo = cache.GetUserUID(ctx); uid == "noUidFound" {
		return user, fmt.Errorf("cannot find user with uid: %s", uid)
	}
	clientToken := ctx.Value(ContextAuthTokenKey).(string)

	cache.usersLock.Lock()
	defer cache.usersLock.Unlock()
	cachedUserData, userDataExists := cache.users[uid] //check if userData cache for user already exists

	// UserDataExists and its valid
	if userDataExists && cachedUserData.isValid() {
		klog.V(5).Info("Using user data from cache.")

		return cachedUserData, nil
	} else {
		// //create metric and set labels
		// HttpDurationByQuery := metric.HttpDurationByLabels(prometheus.Labels{"action": "create_user_cache"})

		// //create timer and return observed duration

		// timer := prometheus.NewTimer(HttpDurationByQuery.WithLabelValues("200")) //change labels
		// defer timer.ObserveDuration()

		if cache.users == nil {
			cache.users = map[string]*UserDataCache{}
		}
		// User not in cache , Initialize and assign to the UID
		user = &UserDataCache{
			userInfo:      userInfo,
			clustersCache: cacheMetadata{ttl: time.Duration(config.Cfg.UserCacheTTL) * time.Millisecond},
			csrCache:      cacheMetadata{ttl: time.Duration(config.Cfg.UserCacheTTL) * time.Millisecond},
			nsrCache:      cacheMetadata{ttl: time.Duration(config.Cfg.UserCacheTTL) * time.Millisecond},
		}
		if cache.users == nil {
			cache.users = map[string]*UserDataCache{}
		}
		cache.users[uid] = user

		// We want to setup the client if passed, this is only for unit tests
		if authzClient != nil {
			user.authzClient = authzClient
		}
	}

	// Before checking each namespace and clusterscoped resource, check if user has access to everything
	userHasAllAccess, err := user.userHasAllAccess(ctx, cache)
	if err != nil {
		klog.Warning("Encountered error while checking if user has access to everything ", err)
	} else {
		if userHasAllAccess {
			klog.V(4).Infof("User %s with uid %s has access to all resources.", userInfo.Username, userInfo.UID)
			return user, nil
		}
		klog.V(5).Infof("User %s with uid %s doesn't have access to all resources. Checking individually",
			userInfo.Username, userInfo.UID)
	}

	userDataCache, err := user.getNamespacedResources(cache, ctx, clientToken)

	// Get cluster scoped resource access for the user.
	if err == nil {
		klog.V(5).Info("No errors on namespacedresources present for: ",
			cache.tokenReviews[clientToken].tokenReview.Status.User.Username)
		userDataCache, err = user.getClusterScopedResources(ctx, cache)
	}
	return userDataCache, err
}

func (user *UserDataCache) userHasAllAccess(ctx context.Context, cache *Cache) (bool, error) {
	impersClientSet := user.getImpersonationClientSet()
	if impersClientSet == nil {
		klog.Warning(impersonationConfigCreationerror)
		return false, errors.New(impersonationConfigCreationerror)
	}
	//If we have a new set of authorized list for the user reset the previous one
	if user.userAuthorizedListSSAR(ctx, impersClientSet, "*", "*") {
		user.csrCache.lock.Lock()
		defer user.csrCache.lock.Unlock()
		user.CsResources = []Resource{{Apigroup: "*", Kind: "*"}}
		user.csrCache.updatedAt = time.Now()

		user.nsrCache.lock.Lock()
		defer user.nsrCache.lock.Unlock()
		user.NsResources = map[string][]Resource{"*": {{Apigroup: "*", Kind: "*"}}}
		user.nsrCache.updatedAt = time.Now()

		cache.shared.mcCache.lock.Lock()
		defer cache.shared.mcCache.lock.Unlock()
		user.ManagedClusters = cache.shared.managedClusters
		user.clustersCache.updatedAt = time.Now()
		user.csrCache.err, user.nsrCache.err, user.clustersCache.err = nil, nil, nil
		return true, nil
	}
	return false, nil
}

// Get a static copy of the current user data. It will use cached data if valid or refresh if needed.
func (cache *Cache) GetUserData(ctx context.Context) (*UserData, error) {
	userDataCache, userDataErr := cache.GetUserDataCache(ctx, nil)

	if userDataErr != nil {
		klog.Error("Error fetching UserAccessData: ", userDataErr)
		return nil, errors.New("unable to resolve query because of error while resolving user's access")
	}
	// Proceed if user's rbac data exists
	// Get a copy of the current user access if user data exists
	userAccess := &UserData{
		CsResources:     userDataCache.GetCsResources(),
		NsResources:     userDataCache.GetNsResources(),
		ManagedClusters: userDataCache.GetManagedClusters(),
	}
	return userAccess, nil
}

// UserCache is valid if the clustersCache, csrCache, and nsrCache are valid
func (user *UserDataCache) isValid() bool {
	return user.csrCache.isValid() && user.nsrCache.isValid() && user.clustersCache.isValid()
}

// Get cluster-scoped resources the user is authorized to list.
// Equivalent to: oc auth can-i list <resource> --as=<user>
func (user *UserDataCache) getClusterScopedResources(ctx context.Context, cache *Cache) (*UserDataCache, error) {
	defer metric.SlowLog("UserDataCache::getClusterScopedResources", 150*time.Millisecond)()

	user.csrCache.err = nil
	user.csrCache.lock.Lock()
	defer user.csrCache.lock.Unlock()

	// Not present in cache, find all cluster scoped resources
	clusterScopedResources := cache.shared.csResourcesMap
	if len(clusterScopedResources) == 0 {
		klog.Warning("Cluster scoped resources from shared cache empty.", user.csrCache.err)
		return user, user.csrCache.err
	}
	impersClientSet := user.getImpersonationClientSet()
	if impersClientSet == nil {
		user.csrCache.err = errors.New(impersonationConfigCreationerror)
		klog.Warning(impersonationConfigCreationerror)
		return user, user.csrCache.err
	}
	// If we have a new set of authorized list for the user reset the previous one
	user.CsResources = nil

	// For each cluster-scoped resource, check if the user is authorized to list.
	// Paralellize SSAR API calls.
	wg := sync.WaitGroup{}
	lock := sync.Mutex{}
	for res := range clusterScopedResources {
		wg.Add(1)
		go func(group, kind string) {
			defer wg.Done()
			if user.userAuthorizedListSSAR(ctx, impersClientSet, group, kind) {
				lock.Lock()
				defer lock.Unlock()
				user.CsResources = append(user.CsResources,
					Resource{Apigroup: group, Kind: kind})
			}
		}(res.Apigroup, res.Kind)
	}
	wg.Wait() // Wait for all requests to complete.

	uid, userInfo := cache.GetUserUID(ctx)
	klog.V(7).Infof("User %s with uid: %s has access to these cluster scoped res: %+v \n", userInfo.Username, uid,
		user.CsResources)
	user.csrCache.updatedAt = time.Now()
	return user, user.csrCache.err
}

func (user *UserDataCache) userAuthorizedListSSAR(ctx context.Context, authzClient v1.AuthorizationV1Interface,
	apigroup string, kind_plural string) bool {
	accessCheck := &authz.SelfSubjectAccessReview{
		Spec: authz.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authz.ResourceAttributes{
				Verb:     "list",
				Group:    apigroup,
				Resource: kind_plural,
			},
		},
	}
	result, err := authzClient.SelfSubjectAccessReviews().Create(ctx, accessCheck, metav1.CreateOptions{})

	if err != nil {
		klog.Error("Error creating SelfSubjectAccessReviews.", err)
	} else {
		klog.V(6).Infof("SelfSubjectAccessReviews API result for resource %s group %s : %v\n",
			kind_plural, apigroup, prettyPrint(result.Status.String()))
		if result.Status.Allowed {
			return true
		}
	}
	return false

}

func (user *UserDataCache) updateUserManagedClusterList(cache *Cache, ns string) {
	user.clustersCache.lock.Lock()
	defer user.clustersCache.lock.Unlock()
	_, managedClusterNs := cache.shared.managedClusters[ns]
	if managedClusterNs {
		if user.ManagedClusters == nil {
			user.ManagedClusters = map[string]struct{}{}
		}
		user.ManagedClusters[ns] = struct{}{}

	}
}

// Request the SelfSubjectRullesRreview(SSRR) for the namespace and process the rules.
func (user *UserDataCache) getSSRRforNamespace(ctx context.Context, cache *Cache, ns string,
	lock *sync.Mutex) {
	// Request the SelfSubjectRulesReview for the namespace.
	rulesCheck := authz.SelfSubjectRulesReview{
		Spec: authz.SelfSubjectRulesReviewSpec{
			Namespace: ns,
		},
	}
	result, err := user.getImpersonationClientSet().SelfSubjectRulesReviews().Create(ctx,
		&rulesCheck, metav1.CreateOptions{})
	if err != nil {
		klog.Error("Error creating SelfSubjectRulesReviews for namespace", err, ns)
	} else {
		klog.V(9).Infof("SelfSubjectRulesReviews Kube API result for ns:%s : %v\n", ns, prettyPrint(result.Status))
	}

	lock.Lock()
	defer lock.Unlock()

	// Process the SSRR result and add to this UserDataCache object.
	for _, rules := range result.Status.ResourceRules {
		for _, verb := range rules.Verbs {
			if verb == "list" || verb == "*" {
				for _, res := range rules.Resources {
					for _, api := range rules.APIGroups {
						// Add the resource if it is not cluster scoped
						// fail-safe mechanism to avoid whitelist - TODO: incorporate whitelist
						if !cache.shared.isClusterScoped(res, api) && (len(rules.ResourceNames) == 0 ||
							(len(rules.ResourceNames) > 0 && rules.ResourceNames[0] == "*")) {
							// if the user has access to all resources, reset userData.NsResources for the namespace
							// No need to loop through all resources. Save the wildcard *
							// exit the resourceRulesLoop
							if res == "*" && api == "*" {
								user.NsResources[ns] = []Resource{{Apigroup: api, Kind: res}}
								klog.V(5).Infof("User %s with uid: %s has access to everything in the namespace %s",
									user.userInfo.Username, user.userInfo.UID, ns)

								// Update user's managedcluster list too as the user has access to everything
								user.updateUserManagedClusterList(cache, ns)
								return
							}
							user.NsResources[ns] = append(user.NsResources[ns],
								Resource{Apigroup: api, Kind: res})
						} else if cache.shared.isClusterScoped(res, api) {
							klog.V(5).Info("Got clusterscoped resource", api, "/",
								res, " from SelfSubjectRulesReviews. Excluding it from ns scoped resoures.")
						} else if len(rules.ResourceNames) > 0 && rules.ResourceNames[0] != "*" {
							klog.V(5).Info("Got whitelist in resourcenames. Excluding resource", api, "/", res,
								" from ns scoped resoures.")
						}
					}
				}
			}
			// Obtain namespaces with create managedclusterveiws resource action
			// Equivalent to: oc auth can-i create ManagedClusterView -n <managedClusterName> --as=<user>
			if verb == "create" || verb == "*" {
				for _, res := range rules.Resources {
					if res == "managedclusterviews" {
						user.updateUserManagedClusterList(cache, ns)
					}
				}
			}
		}
	}
}

// Equivalent to: oc auth can-i --list -n <iterate-each-namespace>
func (user *UserDataCache) getNamespacedResources(cache *Cache, ctx context.Context,
	clientToken string) (*UserDataCache, error) {
	defer metric.SlowLog("UserDataCache::getNamespacedResources", 250*time.Millisecond)()

	// Lock the cache
	user.nsrCache.lock.Lock()
	defer user.nsrCache.lock.Unlock()

	// Clear cached data
	user.nsrCache.err = nil
	user.NsResources = make(map[string][]Resource)
	user.clustersCache.err = nil
	user.ManagedClusters = make(map[string]struct{})

	// get all namespaces from shared cache
	klog.V(5).Info("Getting namespaces from shared cache.")
	allNamespaces, err := cache.shared.getNamespaces(ctx)
	if err != nil || len(allNamespaces) == 0 {
		klog.Warning("All namespaces array from shared cache is empty.", cache.shared.nsCache.err)
		return user, cache.shared.nsCache.err
	}

	// Process each namespace SSRR in an async go routine.
	wg := sync.WaitGroup{}
	lock := sync.Mutex{}
	for _, ns := range allNamespaces {
		wg.Add(1)
		go func(namespace string) {
			defer wg.Done()
			user.getSSRRforNamespace(ctx, cache, namespace, &lock)
		}(ns)
	}
	wg.Wait() // Wait for all go routines to complete.

	uid, userInfo := cache.GetUserUID(ctx)
	klog.V(7).Infof("User %s with uid: %s has access to these namespace scoped res: %+v \n", userInfo.Username, uid,
		user.NsResources)
	klog.V(7).Infof("User %s with uid: %s has access to these ManagedClusters: %+v \n", userInfo.Username, uid,
		user.ManagedClusters)

	user.nsrCache.updatedAt = time.Now()
	user.clustersCache.updatedAt = time.Now()

	return user, user.nsrCache.err
}

// SSRR has resources that are clusterscoped too
func (shared *SharedData) isClusterScoped(kindPlural, apigroup string) bool {
	// lock to prevent checking more than one at a time and check if cluster scoped resources already in cache
	shared.csrCache.lock.Lock()
	defer shared.csrCache.lock.Unlock()
	_, ok := shared.csResourcesMap[Resource{Apigroup: apigroup, Kind: kindPlural}]
	if ok {
		klog.V(9).Info("resource is ClusterScoped ", kindPlural, " ", apigroup, ": ", ok)
	}
	return ok
}

func setImpersonationUserInfo(userInfo authv1.UserInfo) *rest.ImpersonationConfig {
	impersonConfig := &rest.ImpersonationConfig{}
	// All fields in user info, if set, should be added to ImpersonationConfig. Otherwise SSRR won't work.
	// All fields in UserInfo is optional. Set only if there is a value
	//set username
	if userInfo.Username != "" {
		impersonConfig.UserName = userInfo.Username
	}
	//set uid
	if userInfo.UID != "" {
		impersonConfig.UID = userInfo.UID
	}
	//set groups
	if len(userInfo.Groups) > 0 {
		impersonConfig.Groups = userInfo.Groups
	}
	if len(userInfo.Extra) > 0 {
		extraUpdated := map[string][]string{}
		for key, val := range userInfo.Extra {
			extraUpdated[key] = val
		}
		impersonConfig.Extra = extraUpdated //set additional information
	}
	klog.V(9).Info("UserInfo available for impersonation is %+v:", userInfo)
	return impersonConfig
}

// Get a client impersonating the user.
func (user *UserDataCache) getImpersonationClientSet() v1.AuthorizationV1Interface {
	if user.authzClient == nil {
		klog.V(5).Info("Creating New ImpersonationClientSet. ")
		restConfig := config.GetClientConfig()

		// set Impersonation user info
		restConfig.Impersonate = *setImpersonationUserInfo(user.userInfo)
		clientset, err := kubernetes.NewForConfig(restConfig)
		if err != nil {
			klog.Error("Error with creating a new clientset with impersonation config.", err.Error())
			return nil
		}
		user.authzClient = clientset.AuthorizationV1()
	}
	return user.authzClient
}

func (user *UserDataCache) GetCsResources() []Resource {
	user.csrCache.lock.Lock()
	defer user.csrCache.lock.Unlock()
	return user.CsResources
}

func (user *UserDataCache) GetNsResources() map[string][]Resource {
	user.nsrCache.lock.Lock()
	defer user.nsrCache.lock.Unlock()
	return user.NsResources
}

func (user *UserDataCache) GetManagedClusters() map[string]struct{} {
	user.clustersCache.lock.Lock()
	defer user.clustersCache.lock.Unlock()
	return user.ManagedClusters
}
