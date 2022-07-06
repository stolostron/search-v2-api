// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"sync"
	"time"

	"github.com/stolostron/search-v2-api/pkg/config"
	authzv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// Contains data about the resources the user is allowed to access.
type userData struct {
	// clusters     []string              // Managed clusters where the user has view access.
	// csResources  []resource            // Cluster-scoped resources on hub the user has list access.
	nsResources map[string][]resource // Namespaced resources on hub the user has list access.

	//   key:namespace value list of resources

	// Internal fields to manage the cache.
	// clustersErr       error      // Error while updating clusters data.
	// clustersLock      sync.Mutex // Locks when clusters data is being updated.
	// clustersUpdatedAt time.Time  // Time clusters was last updated.
	updatedAt  time.Time  // updated at namespaces authorized.
	namespaces []string   // need to remove
	err        error      // Error while getting user data from cache
	csrErr     error      // Error while updating cluster-scoped resources data.
	csrLock    sync.Mutex // Locks when cluster-scoped resources data is being updated.
	// csrUpdatedAt time.Time  // Time cluster-scoped resources was last updated.
	nsrErr                 error      // Error while updating namespaced resources data.
	nsrLock                sync.Mutex // Locks when namespaced resources data is being updated.
	nsrUpdatedAt           time.Time  // Time namespaced resources was last updated.
	selfsubjectrulesreview *authzv1.SelfSubjectRulesReview

	// authzClient v1.AuthorizationV1Interface
	// impersonate *kubernetes.Interface // client with impersonation config
}

func (cache *Cache) GetUserData(ctx context.Context, clientToken string) (*userData, error) {
	var user userData
	uid := cache.tokenReviews[clientToken].tokenReview.Status.User.UID //get uid from tokenreview
	cache.usersLock.Lock()
	defer cache.usersLock.Unlock()
	cachedUserData, userDataExists := cache.users[uid] //check if userData cache for user already exists
	if userDataExists {
		klog.V(5).Info("Using user data from cache.")
		return cachedUserData, user.err

	}
	// create new instance
	cache.users[uid] = &user
	userData, err := user.getNamespacedResources(cache, ctx, clientToken)
	return userData, err

}

//TODO:need to change this logic to look do same as oc auth can-i --list -n <iterate-each-namespace>
func (user *userData) getNamespacedResources(cache *Cache, ctx context.Context, clientToken string) (*userData, error) {

	//first we check if we already have user's namespaced resources in userData cache
	user.nsrLock.Lock()
	defer user.nsrLock.Unlock()
	if len(user.namespaces) > 0 &&
		time.Now().Before(user.updatedAt.Add(time.Duration(config.Cfg.UserCacheTTL)*time.Millisecond)) {
		klog.V(5).Info("Using user's namespaced resources from cache.")
		return user, user.nsrErr
	}

	// get all namespaces from shared cache:
	klog.V(5).Info("Getting namespaces from shared cache.")
	user.csrLock.Lock()
	defer user.csrLock.Unlock()
	allNamespaces := cache.shared.namespaces
	user.csrErr = nil

	impersClientset := cache.getImpersonationClientSet(clientToken, cache.restConfig)
	user.nsResources = make(map[string][]resource)
	for _, ns := range allNamespaces {

		selfCheck := authzv1.SelfSubjectRulesReview{
			Spec: authzv1.SelfSubjectRulesReviewSpec{
				Namespace: ns,
			},
		}

		result, err := impersClientset.AuthorizationV1().SelfSubjectRulesReviews().Create(ctx, &selfCheck, metav1.CreateOptions{})
		if err != nil {
			klog.Error("Error creating SelfSubjectAccessReviews ", err.Error())
		}

		for _, rules := range result.Status.ResourceRules { //iterate objects
			for _, verb := range rules.Verbs {
				if verb == "list" { //drill down to list only

					for _, res := range rules.Resources {
						for _, api := range rules.APIGroups {
							user.nsResources[ns] = append(user.nsResources[ns], resource{apigroup: api, kind: res}) //cache rules to users's data
						}
					}
				}
			}

		}
	}
	user.nsrUpdatedAt = time.Now()
	return user, user.nsrErr
}

func (cache *Cache) getImpersonationClientSet(clientToken string, config *rest.Config) kubernetes.Interface {

	config.Impersonate = rest.ImpersonationConfig{
		UserName: cache.tokenReviews[clientToken].tokenReview.Status.User.Username,
		UID:      cache.tokenReviews[clientToken].tokenReview.Status.User.UID,
	}

	clientset, err := kubernetes.NewForConfig(cache.restConfig)
	if err != nil {
		klog.Error("Error with creating a new clientset with impersonation config.", err.Error())
	}

	cache.kubeClient = clientset

	return cache.kubeClient
}
