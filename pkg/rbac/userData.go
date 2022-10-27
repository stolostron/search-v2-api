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

var impersonationConfigCreationerror = "Error creating clientset with impersonation config"

// Contains data about the resources the user is allowed to access.
type UserDataCache struct {
	userData UserData

	// Internal fields to manage the cache.
	clustersErr error // Error while updating clusters data.
	// NOTE: not implemented because we use the nsrLock
	// clustersLock      sync.Mutex // Locks when clusters data is being updated.
	clustersUpdatedAt time.Time // Time clusters was last updated.

	csrErr       error      // Error while updating cluster-scoped resources data.
	csrLock      sync.Mutex // Locks when cluster-scoped resources data is being updated.
	csrUpdatedAt time.Time  // Time cluster-scoped resources was last updated.

	nsrErr       error      // Error while updating namespaced resources data.
	nsrLock      sync.Mutex // Locks when namespaced resources data is being updated.
	nsrUpdatedAt time.Time  // Time namespaced resources was last updated.

	authzClient v1.AuthorizationV1Interface
}

// Stuct to keep a copy of users access
type UserData struct {
	CsResources     []Resource            // Cluster-scoped resources on hub the user has list access.
	NsResources     map[string][]Resource // Namespaced resources on hub the user has list access.
	ManagedClusters map[string]struct{}   // Managed clusters where the user has view access.
}

//Get user's UID
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
		if cache.users == nil {
			cache.users = map[string]*UserDataCache{}
		}
		// User not in cache , Initialize and assign to the UID
		user = &UserDataCache{}
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
	userHasAllAccess, err := user.userHasAllAccess(cache, ctx, clientToken)
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
		userDataCache, err = user.getClusterScopedResources(cache, ctx, clientToken)
	}
	return userDataCache, err
}

func (user *UserDataCache) userHasAllAccess(cache *Cache, ctx context.Context, clientToken string) (bool, error) {
	impersClientset, err := user.getImpersonationClientSet(clientToken, cache)
	if err != nil {
		klog.Warning(impersonationConfigCreationerror, err.Error())
		return false, err
	}
	//If we have a new set of authorized list for the user reset the previous one
	if user.userAuthorizedListSSAR(ctx, impersClientset, "*", "*") {
		user.csrLock.Lock()
		defer user.csrLock.Unlock()
		user.userData.CsResources = []Resource{{Apigroup: "*", Kind: "*"}}
		user.csrUpdatedAt = time.Now()

		user.nsrLock.Lock()
		defer user.nsrLock.Unlock()
		user.userData.NsResources = map[string][]Resource{"*": {{Apigroup: "*", Kind: "*"}}}
		user.nsrUpdatedAt = time.Now()

		user.userData.ManagedClusters = cache.shared.managedClusters
		user.clustersUpdatedAt = time.Now()
		user.csrErr, user.nsrErr, user.clustersErr = nil, nil, nil
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

/* Cache is Valid if the csrUpdatedAt and nsrUpdatedAt times are before the
Cache expiry time */
func (user *UserDataCache) isValid() bool {
	if (time.Now().Before(user.csrUpdatedAt.Add(time.Duration(config.Cfg.UserCacheTTL) * time.Millisecond))) &&
		(time.Now().Before(user.nsrUpdatedAt.Add(time.Duration(config.Cfg.UserCacheTTL) * time.Millisecond))) &&
		(time.Now().Before(user.clustersUpdatedAt.Add(time.Duration(config.Cfg.UserCacheTTL) * time.Millisecond))) {
		return true
	}
	return false
}

// Get cluster-scoped resources the user is authorized to list.
// Equivalent to: oc auth can-i list <resource> --as=<user>
func (user *UserDataCache) getClusterScopedResources(cache *Cache, ctx context.Context,
	clientToken string) (*UserDataCache, error) {
	defer metric.SlowLog("UserDataCache::getClusterScopedResources", 150*time.Millisecond)()

	user.csrErr = nil
	user.csrLock.Lock()
	defer user.csrLock.Unlock()

	// Not present in cache, find all cluster scoped resources
	clusterScopedResources := cache.shared.csResourcesMap
	if len(clusterScopedResources) == 0 {
		klog.Warning("Cluster scoped resources from shared cache empty.", user.csrErr)
		return user, user.csrErr
	}
	impersClientset, err := user.getImpersonationClientSet(clientToken, cache)
	if err != nil {
		user.csrErr = err
		klog.Warning(impersonationConfigCreationerror, err.Error())
		return user, user.csrErr
	}
	// If we have a new set of authorized list for the user reset the previous one
	user.userData.CsResources = nil

	// Paralellize API calls.
	var wg sync.WaitGroup
	var lock sync.Mutex
	// For each cluster-scoped resource, check if the user is authorized to list.
	for res := range clusterScopedResources {
		wg.Add(1)
		go func(group, kind string) {
			defer wg.Done()
			if user.userAuthorizedListSSAR(ctx, impersClientset, group, kind) {
				lock.Lock()
				defer lock.Unlock()
				user.userData.CsResources = append(user.userData.CsResources,
					Resource{Apigroup: group, Kind: kind})
			}
		}(res.Apigroup, res.Kind)
	}
	wg.Wait() // Wait for all requests to complete.

	uid, userInfo := cache.GetUserUID(ctx)
	klog.V(7).Infof("User %s with uid: %s has access to these cluster scoped res: %+v \n", userInfo.Username, uid,
		user.userData.CsResources)
	user.csrUpdatedAt = time.Now()
	return user, user.csrErr
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
	_, managedClusterNs := cache.shared.managedClusters[ns]
	if managedClusterNs {
		if user.userData.ManagedClusters == nil {
			user.userData.ManagedClusters = map[string]struct{}{}
		}
		user.userData.ManagedClusters[ns] = struct{}{}

	}
}

// Equivalent to: oc auth can-i --list -n <iterate-each-namespace>
func (user *UserDataCache) getNamespacedResources(cache *Cache, ctx context.Context,
	clientToken string) (*UserDataCache, error) {
	defer metric.SlowLog("UserDataCache::getNamespacedResources", 200*time.Millisecond)()
	// check if we already have user's namespaced resources in userData cache and check if time is expired
	user.nsrLock.Lock()
	defer user.nsrLock.Unlock()

	// clear cache
	user.csrErr = nil
	user.userData.NsResources = nil
	user.clustersErr = nil
	user.userData.ManagedClusters = nil

	// get all namespaces from shared cache:
	klog.V(5).Info("Getting namespaces from shared cache.")
	user.csrLock.Lock()
	defer user.csrLock.Unlock()
	allNamespaces := cache.shared.namespaces
	if len(allNamespaces) == 0 {
		klog.Warning("All namespaces array from shared cache is empty.", cache.shared.nsErr)
		return user, cache.shared.nsErr
	}

	impersClientset, err := user.getImpersonationClientSet(clientToken, cache)
	if err != nil {
		klog.Warning(impersonationConfigCreationerror, err.Error())
		return user, err
	}

	user.userData.NsResources = make(map[string][]Resource)
	user.userData.ManagedClusters = make(map[string]struct{})
	uid, userInfo := cache.GetUserUID(ctx)

	for _, ns := range allNamespaces {
		rulesCheck := authz.SelfSubjectRulesReview{
			Spec: authz.SelfSubjectRulesReviewSpec{
				Namespace: ns,
			},
		}
		result, err := impersClientset.SelfSubjectRulesReviews().Create(ctx, &rulesCheck, metav1.CreateOptions{})
		if err != nil {
			klog.Error("Error creating SelfSubjectRulesReviews for namespace", err, ns)
		} else {
			klog.V(9).Infof("SelfSubjectRulesReviews Kube API result for ns:%s : %v\n", ns, prettyPrint(result.Status))
		}

		// Name the loop - to break out of the loop if user has access to everything (*)
	resourceRulesLoop:
		for _, rules := range result.Status.ResourceRules {
			for _, verb := range rules.Verbs {
				if verb == "list" || verb == "*" {
					for _, res := range rules.Resources {
						for _, api := range rules.APIGroups {
							//Add the resource if it is not cluster scoped
							//fail-safe mechanism to avoid whitelist - TODO: incorporate whitelist
							if !cache.shared.isClusterScoped(res, api) && (len(rules.ResourceNames) == 0 ||
								(len(rules.ResourceNames) > 0 && rules.ResourceNames[0] == "*")) {
								// if the user has access to all resources, reset userData.NsResources for the namespace
								// No need to loop through all resources. Save the wildcard *
								// exit the resourceRulesLoop
								if res == "*" && api == "*" {
									user.userData.NsResources[ns] = []Resource{{Apigroup: api, Kind: res}}
									klog.V(5).Infof("User %s with uid: %s has access to everything in the namespace %s",
										userInfo.Username, userInfo.UID, ns)

									// Update user's managedcluster list too as the user has access to everything
									user.updateUserManagedClusterList(cache, ns)

									break resourceRulesLoop
								}
								user.userData.NsResources[ns] = append(user.userData.NsResources[ns],
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
	klog.V(7).Infof("User %s with uid: %s has access to these namespace scoped res: %+v \n", userInfo.Username, uid,
		user.userData.NsResources)
	klog.V(7).Infof("User %s with uid: %s has access to these ManagedClusters: %+v \n", userInfo.Username, uid,
		user.userData.ManagedClusters)

	user.nsrUpdatedAt = time.Now()
	user.clustersUpdatedAt = time.Now()

	return user, user.nsrErr
}

//SSRR has resources that are clusterscoped too
func (shared *SharedData) isClusterScoped(kindPlural, apigroup string) bool {
	// lock to prevent checking more than one at a time and check if cluster scoped resources already in cache
	shared.csLock.Lock()
	defer shared.csLock.Unlock()
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

func (user *UserDataCache) getImpersonationClientSet(clientToken string, cache *Cache) (v1.AuthorizationV1Interface,
	error) {

	if user.authzClient == nil {
		klog.V(5).Info("Creating New ImpersonationClientSet. ")
		restConfig := config.GetClientConfig()
		trUser := cache.tokenReviews[clientToken].tokenReview.Status.User
		// set Impersonation user info
		restConfig.Impersonate = *setImpersonationUserInfo(trUser)

		clientset, err := kubernetes.NewForConfig(restConfig)
		if err != nil {
			klog.Error("Error with creating a new clientset with impersonation config.", err.Error())
			return nil, err
		}
		user.authzClient = clientset.AuthorizationV1()
	}
	return user.authzClient, nil
}

func (user *UserDataCache) GetCsResources() []Resource {
	return user.userData.CsResources
}

func (user *UserDataCache) GetNsResources() map[string][]Resource {
	return user.userData.NsResources
}

func (user *UserDataCache) GetManagedClusters() map[string]struct{} {
	return user.userData.ManagedClusters
}
