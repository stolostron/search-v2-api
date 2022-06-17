// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"sync"
	"time"

	"github.com/stolostron/search-v2-api/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

type userData struct {
	err        error
	namespaces []string
	//namespacedResourceRules map[string]map[string]map[string]string //["namespace":["group1": ["kind", "kind2",..]], "namespace1": ["group1" : ["kind1", "kind2"]]] ?
	updatedAt time.Time
	lock      sync.Mutex
}

func (cache *Cache) NamespacedResources(ctx context.Context, clientToken string) ([]string, error) {

	uid := cache.tokenReviews[clientToken].tokenReview.UID

	namespaced, err := cache.users[string(uid)].getNamespacedResources(cache, ctx, string(uid))
	return namespaced, err

}

func (users *userData) getNamespacedResources(cache *Cache, ctx context.Context, uid string) ([]string, error) {

	users.lock.Lock()
	defer users.lock.Unlock()
	if users.namespaces != nil &&
		time.Now().Before(users.updatedAt.Add(time.Duration(config.Cfg.UserCacheTTL)*time.Millisecond)) {
		klog.V(5).Info("Using user's namespaced resources from cache.")
		return users.namespaces, users.err

	}

	klog.V(5).Info("Getting namespaced resources from ")
	// err := getResources(dynamic, ctx,
	// 	"apps", "v1", "deployments", namespace)

	nsrs, err := getResources(ctx) //here we get the namespaces
	if err != nil {
		klog.Errorf("Error getting namespaces", err)
	}
	users.namespaces = nsrs

	return users.namespaces, users.err
}

func getResources(ctx context.Context) ([]string, error) {

	//	dynamic := config.GetDynamicClient() //create a dynamic client to perform RESTful operations on api resources.

	// supportedResources := []*metav1.APIResourceList{}
	// namespaceResourceMap := make(map[string]map[string]string) //create a map

	var listns []string

	nslist, err := config.KubeClient().CoreV1().Namespaces().List(ctx, metav1.ListOptions{}) // get all namespaces in cluster
	if err != nil {
		klog.Warning("Namespaces could not be listed: ", err)
		return nil, err
	}

	// create namepsace list:
	for _, n := range nslist.Items {
		listns = append(listns, n.Name)
	}

	// apiResources, err := config.KubeClient().ServerPreferredResources() //get all resources
	// if err != nil && apiResources == nil {                              // only return if the list is empty
	// 	return nil, err
	// } else if err != nil {
	// 	klog.Warning("ServerPreferredResources could not list all available resources: ", err)
	// }

	// //iterate through all namespaces in list and for each namespace find resources and append to map:
	// for _, ns := range listns {

	// 	for _, apiList := range apiResources {

	// 		// apiList.
	// 		for _, apiResource := range apiList.APIResources {
	// 			for _, verb := range apiResource.Verbs {
	// 				if verb == "list" {

	// 					//get all resources that have list verb:
	// 					if apiResource.Namespaced {

	// 						// fmt.Println("Namespace:", ns)
	// 						// fmt.Println("APIRESOURCES", apiResource.Kind, apiResource.Group)

	// 						// namespaceResourceMap[ns] =
	// 						namespaceResourceMap[ns] = apiResource.Group[apiResource.Kind]
	// 						// resourceId := schema.GroupVersionResource{
	// 						// 	Group:    apiResource.Group,
	// 						// 	Version:  apiResource.Version,
	// 						// 	Resource: apiResource.Name,
	// 						// }

	// 						// list, _ := dynamic.Resource(resourceId).Namespace(ns).
	// 						// 	List(ctx, metav1.ListOptions{})

	// 						// fmt.Println(list.Items)

	// 					}

	// 					// namespaceResourceMap[ns] = list.Items
	// 					// supportedNamepacedResources = append(supportedNamepacedResources, apiResource)
	// 				}
	// 				// } else {
	// 				//  supportedClusterScopedResources = append(supportedClusterScopedResources, apiResource)
	// 				// }
	// 			}
	// 		}
	// 	}
	// }

	return listns, err

}
