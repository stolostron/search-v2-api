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
	authzv1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	v1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// Contains data about the resources the user is allowed to access.
type userData struct {
	// clusters     []string              // Managed clusters where the user has view access.
	csResources []resource            // Cluster-scoped resources on hub the user has list access.
	nsResources map[string][]resource // Namespaced resources on hub the user has list access.

	// Internal fields to manage the cache.
	// clustersErr       error      // Error while updating clusters data.
	// clustersLock      sync.Mutex // Locks when clusters data is being updated.
	// clustersUpdatedAt time.Time  // Time clusters was last updated.
	err          error      // Error while getting user data from cache
	csrErr       error      // Error while updating cluster-scoped resources data.
	csrLock      sync.Mutex // Locks when cluster-scoped resources data is being updated.
	csrUpdatedAt time.Time  // Time cluster-scoped resources was last updated.
	nsrErr       error      // Error while updating namespaced resources data.
	nsrLock      sync.Mutex // Locks when namespaced resources data is being updated.
	nsrUpdatedAt time.Time  // Time namespaced resources was last updated.
	authzClient  authzv1.AuthorizationV1Interface
	// impersonate *kubernetes.Interface // client with impersonation config
}

func (cache *Cache) GetUserData(ctx context.Context, clientToken string) (*userData, error) {
	var user userData
	uid := cache.tokenReviews[clientToken].tokenReview.Status.User.UID //get uid from tokenreview
	cache.usersLock.Lock()
	defer cache.usersLock.Unlock()
	cachedUserData, userDataExists := cache.users[uid] //check if userData cache for user already exists
	if userDataExists && time.Now().Before(cachedUserData.csrUpdatedAt.Add(time.Duration(config.Cfg.UserCacheTTL)*time.Millisecond)) {
		klog.V(5).Info("Using user data from cache.")
		return cachedUserData, user.err
	}

	userData, err := user.getNamespacedResources(cache, ctx, clientToken)

	// Get cluster scoped resources for the user
	if err == nil {
		klog.Info("No errors on namespacedresources not present ....Computing now")
		userData, err = user.getClusterScopedResources(cache, ctx, clientToken)
	} else {
		klog.Info("Errors on namespacedresources not present ....Computing now")
	}

	return userData, err

}

// The following achieves same result as oc auth can-i list <resource> --as=<user>
func (user *userData) getClusterScopedResources(cache *Cache, ctx context.Context, clientToken string) (*userData, error) {

	// get all cluster scoped from shared cache:
	klog.V(5).Info("Getting cluster scoped resources from shared cache.")
	user.csrErr = nil
	user.csrLock.Lock()
	defer user.csrLock.Unlock()

	//Check if the user has the Cluster scoped resources in cache
	if len(user.csResources) > 0 &&
		time.Now().Before(user.csrUpdatedAt.Add(time.Duration(config.Cfg.UserCacheTTL)*time.Millisecond)) {
		klog.V(5).Info("Using user's cluster scoped resources from cache.")
		user.csrErr = nil
		return user, user.csrErr
	}
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
	for _, res := range clusterScopedResources {
		if user.userAuthorizedListCSResource(ctx, impersClientset, res.apigroup, res.kind) {
			user.csResources = append(user.csResources, resource{apigroup: res.apigroup, kind: res.kind})
		}
	}
	user.csrUpdatedAt = time.Now()
	return user, user.csrErr
}

func (user *userData) userAuthorizedListCSResource(ctx context.Context, authzClient v1.AuthorizationV1Interface, apigroup string, kind_plural string) bool {
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
		klog.Error("Error creating SelfSubjectAccessReviews.", err, apigroup, ":", kind_plural)
	} else {
		klog.V(5).Infof("SelfSubjectAccessReviews API result for resource %s group %s : %v\n", kind_plural, apigroup, prettyPrint(result.Status.String()))
		if result.Status.Allowed {
			return true
		}
	}
	return false
}

// The following achieves same result as oc auth can-i --list -n <iterate-each-namespace>
func (user *userData) getNamespacedResources(cache *Cache, ctx context.Context, clientToken string) (*userData, error) {

	//first we check if we already have user's namespaced resources in userData cache
	user.nsrLock.Lock()
	defer user.nsrLock.Unlock()
	if len(user.nsResources) > 0 &&
		time.Now().Before(user.nsrUpdatedAt.Add(time.Duration(config.Cfg.UserCacheTTL)*time.Millisecond)) {
		klog.V(5).Info("Using user's namespaced resources from cache.")
		user.nsrErr = nil
		return user, user.nsrErr
	}

	// get all namespaces from shared cache:
	klog.V(5).Info("Getting namespaces from shared cache.")
	user.csrLock.Lock()
	defer user.csrLock.Unlock()
	allNamespaces := cache.shared.namespaces
	if len(allNamespaces) == 0 {
		klog.Warning("All namespaces array from shared cache is empty.", cache.shared.nsErr)
		return user, cache.shared.nsErr
	}
	// allNamespaces = removeDuplicateStr(allNamespaces)
	user.csrErr = nil

	impersClientset, err := user.getImpersonationClientSet(clientToken, cache)
	if err != nil {
		klog.Warning("Error creating clientset with impersonation config.", err.Error())
		return user, err
	}

	user.nsResources = make(map[string][]resource)

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
		} else {
			klog.V(9).Infof("TokenReview Kube API result: %v\n", prettyPrint(result.Status))
		}
		for _, rules := range result.Status.ResourceRules { //iterate objects
			for _, verb := range rules.Verbs {
				if verb == "list" || verb == "*" { //drill down to list only

					for _, res := range rules.Resources {
						for _, api := range rules.APIGroups {
							user.nsResources[ns] = append(user.nsResources[ns], resource{apigroup: api, kind: res})
						}
					}
				}
			}

		}
	}

	user.nsrUpdatedAt = time.Now()
	return user, user.nsrErr
}

func (user *userData) getImpersonationClientSet(clientToken string, cache *Cache) (v1.AuthorizationV1Interface,
	error) {

	if user.authzClient == nil {

		cache.restConfig.Impersonate = rest.ImpersonationConfig{
			UserName: cache.tokenReviews[clientToken].tokenReview.Status.User.Username,
			UID:      cache.tokenReviews[clientToken].tokenReview.Status.User.UID,
		}

		clientset, err := kubernetes.NewForConfig(cache.restConfig)
		if err != nil {
			klog.Error("Error with creating a new clientset with impersonation config.", err.Error())
			return nil, err
		}

		user.authzClient = clientset.AuthorizationV1()

	}

	return user.authzClient, nil
}

// //helper function to remove duplicates from shared resources list:
// func removeDuplicateStr(strSlice []string) []string {
// 	allKeys := make(map[string]bool)
// 	list := []string{}
// 	for _, item := range strSlice {
// 		if _, value := allKeys[item]; !value {
// 			allKeys[item] = true
// 			list = append(list, item)
// 		}
// 	}
// 	return list
// }
