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
	clusters    []string              // Managed clusters where the user has view access.

	// Internal fields to manage the cache.
	clustersErr error // Error while updating clusters data.
	// clustersLock      sync.Mutex // Locks when clusters data is being updated. NOTE: not implmented because we use the nsrLock
	clustersUpdatedAt time.Time // Time clusters was last updated.

	csrErr       error      // Error while updating cluster-scoped resources data.
	csrLock      sync.Mutex // Locks when cluster-scoped resources data is being updated.
	csrUpdatedAt time.Time  // Time cluster-scoped resources was last updated.

	nsrErr       error      // Error while updating namespaced resources data.
	nsrLock      sync.Mutex // Locks when namespaced resources data is being updated.
	nsrUpdatedAt time.Time  // Time namespaced resources was last updated.

	authzClient v1.AuthorizationV1Interface
}

func (cache *Cache) GetUserData(ctx context.Context, clientToken string,
	authzClient v1.AuthorizationV1Interface) {
	var user *userData
	uid := cache.tokenReviews[clientToken].tokenReview.Status.User.UID //get uid from tokenreview
	cache.usersLock.Lock()
	defer cache.usersLock.Unlock()
	cachedUserData, userDataExists := cache.users[uid] //check if userData cache for user already exists
	// UserDataExists and its valid
	if userDataExists && userCacheValid(cachedUserData) {
		klog.V(5).Info("Using user data from cache.")
		return //cachedUserData, nil
	} else {
		// User not in cache , Initialize and assign to the UID
		user = &userData{}
		cache.users[uid] = user
		// We want to setup the client if passed, this is only for unit tests
		if authzClient != nil {
			user.authzClient = authzClient
		}
	}

	defer func(start time.Time) {
		klog.Infof("\t%+v \t- GetUserData()", time.Since(start))
	}(time.Now())
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		user.getNamespacedResources(cache, ctx, clientToken)
	}()

	go func() {
		defer wg.Done()
		user.getClusterScopedResources(cache, ctx, clientToken)
	}()

	wg.Wait()

	// Get cluster scoped resources for the user
	// TO DO : Make this parallel operation
	// if err == nil {
	// 	klog.V(5).Info("No errors on namespacedresources present for: ",
	// 		cache.tokenReviews[clientToken].tokenReview.Status.User.Username)
	// 	userData, err = user.getClusterScopedResources(cache, ctx, clientToken)
	// }

	// return userData, err

}

/* Cache is Valid if the csrUpdatedAt and nsrUpdatedAt times are before the
Cache expiry time */
func userCacheValid(user *userData) bool {
	if (time.Now().Before(user.csrUpdatedAt.Add(time.Duration(config.Cfg.UserCacheTTL) * time.Millisecond))) &&
		(time.Now().Before(user.nsrUpdatedAt.Add(time.Duration(config.Cfg.UserCacheTTL) * time.Millisecond))) &&
		(time.Now().Before(user.clustersUpdatedAt.Add(time.Duration(config.Cfg.UserCacheTTL) * time.Millisecond))) {
		return true
	}
	return false
}

// Equivalent to: oc auth can-i list <resource> --as=<user>
func (user *userData) getClusterScopedResources(cache *Cache, ctx context.Context,
	clientToken string) (*userData, error) {
	defer func(start time.Time) {
		klog.Infof("\t%+v \t- getClusterScopedResources()", time.Since(start))
	}(time.Now())

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
	wg := &sync.WaitGroup{}
	for _, res := range clusterScopedResources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if user.userAuthorizedListCSResource(ctx, impersClientset, res.apigroup, res.kind) {
				user.csResources = append(user.csResources, resource{apigroup: res.apigroup, kind: res.kind})
			}
		}()
	}
	wg.Wait()
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

// Equivalent to: oc auth can-i --list -n <iterate-each-namespace>
func (user *userData) getNamespacedResources(cache *Cache, ctx context.Context, clientToken string) (*userData, error) {
	defer func(start time.Time) {
		klog.Infof("\t%+v \t- getNamespacedResources()", time.Since(start))
	}(time.Now())

	// check if we already have user's namespaced resources in userData cache and check if time is expired
	user.nsrLock.Lock()
	defer user.nsrLock.Unlock()

	// clear cache
	user.csrErr = nil
	user.nsResources = nil
	user.clustersErr = nil
	user.clusters = nil

	// get all namespaces from shared cache:

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

	user.nsResources = make(map[string][]resource)
	managedClusters := cache.shared.managedClusters

	lock := sync.Mutex{}
	wg := &sync.WaitGroup{}
	for _, ns := range allNamespaces {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rulesCheck := authz.SelfSubjectRulesReview{
				Spec: authz.SelfSubjectRulesReviewSpec{
					Namespace: ns,
				},
			}
			result, err := impersClientset.SelfSubjectRulesReviews().Create(ctx, &rulesCheck, metav1.CreateOptions{})
			if err != nil {
				klog.Error("Error creating SelfSubjectRulesReviews for namespace", err, ns)
			} else {
				klog.V(9).Infof("SelfSubjectRulesReviews Kube API result: %v\n", prettyPrint(result.Status))
			}
			for _, rules := range result.Status.ResourceRules {
				for _, verb := range rules.Verbs {
					if verb == "list" || verb == "*" { //TODO: resourceName == "*" && verb == "*" then exit loop
						for _, res := range rules.Resources {
							for _, api := range rules.APIGroups {
								lock.Lock()
								user.nsResources[ns] = append(user.nsResources[ns], resource{apigroup: api, kind: res})
								lock.Unlock()
							}
						}
					}
					// Obtain namespaces with create managedclusterveiws resource action
					// Equivalent to: oc auth can-i create ManagedClusterView -n <managedClusterName> --as=<user>

					if verb == "create" || verb == "*" {
						for _, res := range rules.Resources {
							if res == "managedclusterviews" {
								for i := range managedClusters {
									if managedClusters[i] == ns {
										user.clusters = append(user.clusters, ns)
									}
								}
							}

						}
					}

				}

			}
		}()
	}
	wg.Wait()

	user.nsrUpdatedAt = time.Now()
	user.clustersUpdatedAt = time.Now()

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
