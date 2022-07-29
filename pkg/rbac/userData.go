// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"sync"
	"time"

	"github.com/stolostron/search-v2-api/pkg/config"
	authz "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// Contains data about the resources the user is allowed to access.
type userData struct {
	csResources []resource            // Cluster-scoped resources on hub the user has list access.
	nsResources map[string][]resource // Namespaced resources on hub the user has list access.

	// Internal fields to manage the cache.
	csrErr       error      // Error while updating cluster-scoped resources data.
	csrLock      sync.Mutex // Locks when cluster-scoped resources data is being updated.
	csrUpdatedAt time.Time  // Time cluster-scoped resources was last updated.

	nsrErr       error      // Error while updating namespaced resources data.
	nsrLock      sync.Mutex // Locks when namespaced resources data is being updated.
	nsrUpdatedAt time.Time  // Time namespaced resources was last updated.

	authzClient v1.AuthorizationV1Interface
}

func (cache *Cache) GetUserData(ctx context.Context, clientToken string,
	authzClient v1.AuthorizationV1Interface) (*userData, error) {
	var user *userData
	uid := cache.tokenReviews[clientToken].tokenReview.Status.User.UID //get uid from tokenreview
	cache.usersLock.Lock()
	defer cache.usersLock.Unlock()
	cachedUserData, userDataExists := cache.users[uid] //check if userData cache for user already exists
	// UserDataExists and its valid
	if userDataExists && userCacheValid(cachedUserData) {
		klog.V(5).Info("Using user data from cache.")
		return cachedUserData, nil
	} else {
		// User not in cache , Initialize and assign to the UID
		user = &userData{}
		cache.users[uid] = user
		// We want to setup the client if passed, this is only for unit tests
		if authzClient != nil {
			user.authzClient = authzClient
		}
	}
	userData, err := user.getNamespacedResources(cache, ctx, clientToken)

	// Get cluster scoped resources for the user
	// TO DO : Make this parallel operation
	if err == nil {
		klog.V(5).Info("No errors on namespacedresources present for: ",
			cache.tokenReviews[clientToken].tokenReview.Status.User.Username)
		userData, err = user.getClusterScopedResources(cache, ctx, clientToken)
	}

	//call managedCluster here
	return userData, err

}

/* Cache is Valid if the csrUpdatedAt and nsrUpdatedAt times are before the
Cache expiry time */
func userCacheValid(user *userData) bool {
	if (time.Now().Before(user.csrUpdatedAt.Add(time.Duration(config.Cfg.UserCacheTTL) * time.Millisecond))) &&
		(time.Now().Before(user.nsrUpdatedAt.Add(time.Duration(config.Cfg.UserCacheTTL) * time.Millisecond))) {
		return true
	}
	return false
}

// The following achieves same result as oc auth can-i list <resource> --as=<user>
func (user *userData) getClusterScopedResources(cache *Cache, ctx context.Context,
	clientToken string) (*userData, error) {

	// get all cluster scoped from shared cache:
	klog.V(5).Info("Getting cluster scoped resources from shared cache.")
	user.csrErr = nil
	user.csrLock.Lock()
	defer user.csrLock.Unlock()

	// Not present in cache, find all cluster scoped resources
	clusterScopedResources := cache.shared.csResources
	if len(clusterScopedResources) == 0 {
		klog.Warning("Cluster scoped resources from shared cache empty.", user.csrErr)
		return user, user.csrErr
	}
	impersClientset, err := user.getImpersonationClientSet(clientToken, cache)
	if err != nil {
		user.csrErr = err
		klog.Warning("Error creating clientset with impersonation config.", err.Error())
		return user, user.csrErr
	}
	//If we have a new set of authorized list for the user reset the previous one
	user.csResources = nil
	for _, res := range clusterScopedResources {
		if user.userAuthorizedListCSResource(ctx, impersClientset, res.apigroup, res.kind) {
			user.csResources = append(user.csResources, resource{apigroup: res.apigroup, kind: res.kind})
		}
	}
	user.csrUpdatedAt = time.Now()
	return user, user.csrErr
}

func (user *userData) userAuthorizedListCSResource(ctx context.Context, authzClient v1.AuthorizationV1Interface,
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
		klog.V(5).Infof("SelfSubjectAccessReviews API result for resource %s group %s : %v\n",
			kind_plural, apigroup, prettyPrint(result.Status.String()))
		if result.Status.Allowed {
			return true
		}
	}
	return false

}

// The following achieves same result as oc auth can-i --list -n <iterate-each-namespace>
func (user *userData) getNamespacedResources(cache *Cache, ctx context.Context, clientToken string) (*userData, error) {

	// check if we already have user's namespaced resources in userData cache and check if time is expired
	user.nsrLock.Lock()
	defer user.nsrLock.Unlock()

	// clear cache
	user.csrErr = nil
	user.nsResources = nil

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
		klog.Warning("Error creating clientset with impersonation config.", err.Error())
		return user, err
	}

	nsResourcesAuth := make(map[string][]resource)

	for _, ns := range allNamespaces {
		//
		rulesCheck := authz.SelfSubjectRulesReview{
			Spec: authz.SelfSubjectRulesReviewSpec{
				Namespace: ns,
			},
		}
		result, err := impersClientset.SelfSubjectRulesReviews().Create(ctx, &rulesCheck, metav1.CreateOptions{})
		if err != nil {
			klog.Error("Error creating SelfSubjectRulesReviews for namespace", err, ns)
			user.nsrErr = err
			return user, user.nsrErr
		} else {
			klog.V(9).Infof("TokenReview Kube API result: %v\n", prettyPrint(result.Status))
		}
		for _, rules := range result.Status.ResourceRules { //iterate objects
			for _, verb := range rules.Verbs {
				if verb == "list" || verb == "*" { //drill down to list only
					for _, res := range rules.Resources {
						for _, api := range rules.APIGroups {
							nsResourcesAuth[ns] = append(nsResourcesAuth[ns], resource{apigroup: api, kind: res})
						}
					}
				}
			}

		}
	}

	//take intersection of authorizaed user namespaces and managed clusters:
	managedClusters := cache.shared.managedClusters
	namespaceNames := make([]string, len(nsResourcesAuth))
	for name := range nsResourcesAuth {
		namespaceNames = append(namespaceNames, name)
	}

	namespaceInt, err := intersection(namespaceNames, managedClusters)
	if err != nil {
		klog.Warning("Error getting intersection of resources", err)
		return user, user.nsrErr
	}

	nsResources := make(map[string][]resource)
	for _, v := range namespaceInt {
		if val, ok := nsResourcesAuth[v]; ok {
			nsResources[v] = val
		}
	}

	user.nsResources = nsResources
	user.nsrUpdatedAt = time.Now()

	return user, user.nsrErr
}

func (user *userData) getImpersonationClientSet(clientToken string, cache *Cache) (v1.AuthorizationV1Interface,
	error) {
	if user.authzClient == nil {
		klog.V(5).Info("Creating New ImpersonationClientSet. ")
		restConfig := config.GetClientConfig()
		restConfig.Impersonate = rest.ImpersonationConfig{
			UserName: cache.tokenReviews[clientToken].tokenReview.Status.User.Username,
			UID:      cache.tokenReviews[clientToken].tokenReview.Status.User.UID,
		}
		clientset, err := kubernetes.NewForConfig(restConfig)
		if err != nil {
			klog.Error("Error with creating a new clientset with impersonation config.", err.Error())
			return nil, err
		}
		user.authzClient = clientset.AuthorizationV1()
	}
	return user.authzClient, nil
}

//helper funtion to get intersection:
func intersection(a1, a2 []string) ([]string, error) {
	var intersection []string
	for _, x := range a1 {
		ok := false
		for _, y := range a2 {
			if x == y {
				ok = true
				break
			}
		}
		if ok {
			intersection = append(intersection, x)
		}
	}
	return intersection, nil
}
