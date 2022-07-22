// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/stolostron/search-v2-api/pkg/config"
	authzv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// Contains data about the resources the user is allowed to access.
type userData struct {
	clusters []string // Managed clusters where the user has view access.
	// csResources  []resource            // Cluster-scoped resources on hub the user has list access. //do we need this?
	nsResources map[string][]resource // Namespaced resources on hub the user has list access.

	// Internal fields to manage the cache.
	clustersErr       error      // Error while updating clusters data.
	clustersLock      sync.Mutex // Locks when clusters data is being updated.
	clustersUpdatedAt time.Time  // Time clusters was last updated.
	err               error      // Error while getting user data from cache
	csrErr            error      // Error while updating cluster-scoped resources data.
	csrLock           sync.Mutex // Locks when cluster-scoped resources data is being updated.
	// csrUpdatedAt time.Time  // Time cluster-scoped resources was last updated.
	nsrErr       error      // Error while updating namespaced resources data.
	nsrLock      sync.Mutex // Locks when namespaced resources data is being updated.
	nsrUpdatedAt time.Time  // Time namespaced resources was last updated.

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

	userData, err := user.getNamespacedResources(cache, ctx, clientToken)
	return userData, err

}

// The following achieves same result as oc auth can-i --list -n <iterate-each-namespace>
func (user *userData) getNamespacedResources(cache *Cache, ctx context.Context, clientToken string) (*userData, error) {

	// getting the managed clusters
	managedClusters, _ := user.getManagedClusterResources(ctx, cache, clientToken)

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

	//get all managed clusters:
	user.getManagedClusterResources(ctx, cache, clientToken)

	// allNamespaces = removeDuplicateStr(allNamespaces)
	user.csrErr = nil

	impersClientset, err := cache.getImpersonationClientSet(clientToken, cache.restConfig)
	if err != nil {
		klog.Warning("Error creating clientset with impersonation config.", err.Error())
		return user, err
	}

	user.nsResources = make(map[string][]resource)

	allNamespaces = append(allNamespaces, managedClusters...) //list of all managed clusters and namespaces per user

	for _, ns := range allNamespaces {
		//
		rulesCheck := authzv1.SelfSubjectRulesReview{
			Spec: authzv1.SelfSubjectRulesReviewSpec{
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

func (cache *Cache) getImpersonationClientSet(clientToken string, config *rest.Config) (v1.AuthorizationV1Interface,
	error) {

	if cache.authzClient == nil {

		config.Impersonate = rest.ImpersonationConfig{
			UserName: cache.tokenReviews[clientToken].tokenReview.Status.User.Username,
			UID:      cache.tokenReviews[clientToken].tokenReview.Status.User.UID,
		}

		clientset, err := kubernetes.NewForConfig(cache.restConfig)
		if err != nil {
			klog.Error("Error with creating a new clientset with impersonation config.", err.Error())
			return nil, err
		}

		cache.kubeClient = clientset
		cache.authzClient = clientset.AuthorizationV1()

	}

	return cache.authzClient, nil
}

//For resources in the Managed Clusters search will show resources only if the user is authorized to see the managed cluster
func (user *userData) getManagedClusterResources(ctx context.Context, cache *Cache, clientToken string) ([]string, error) {

	// clusters lock
	user.clustersLock.Lock()
	defer user.clustersLock.Unlock()

	// check to see if we have any clusters in cache and if the update time has not expired
	if len(user.clusters) > 0 &&
		time.Now().Before(user.clustersUpdatedAt.Add(time.Duration(config.Cfg.UserCacheTTL)*time.Millisecond)) {
		klog.V(5).Info("Using user's managed clusters from cache.")
		user.clustersErr = nil
		return user.clusters, user.err
	}

	//get user's managed clusters and cache..
	klog.V(5).Info("Getting managed clusters from Kube Client..")

	// create a kubeclient (TODO: this we already do for the user so we should use the cached client in cache.client)
	cache.restConfig = config.GetClientConfig()
	clientset, err := kubernetes.NewForConfig(cache.restConfig)
	if err != nil {
		klog.Warning("Error with creating a new clientset.", err.Error())

	}
	// get namespacelist (TODO:this we already do above so we can combine (we don't need whole new function))
	namespaceList, _ := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})

	var managedClusterNamespaces []string
	for _, namespace := range namespaceList.Items {

		for _, labels := range namespace.Labels {
			if strings.Contains(labels, "managedCluster") {
				fmt.Println("This label contains managedCluster:", namespace.Name)
				user.clusters = append(user.clusters, namespace.Name)
				managedClusterNamespaces = append(managedClusterNamespaces, namespace.Name)

				break
			}
		}
		// fmt.Println(namespace.ObjectMeta.Labels)

	}

	user.clustersErr = nil

	return managedClusterNamespaces, user.clustersErr
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
