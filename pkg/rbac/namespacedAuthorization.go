package rbac

import (
	"context"
	"fmt"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stolostron/search-v2-api/pkg/config"
	"k8s.io/klog/v2"
)

type namespacedResources struct {
	err       error
	resources map[string]map[string]map[string]string //["namespace":["group1": ["kind", "kind2",..]], "namespace1": ["group1" : ["kind1", "kind2"]]]
	updatedAt time.Time
	lock      sync.Mutex
}

func (cache *Cache) NamespacedResources(ctx context.Context) (map[string]map[string]map[string]string, error) {
	namespaced, err := cache.users.getNamespacedResources(cache, ctx)
	return namespaced, err

}

func (users *namespacedResources) getNamespacedResources(cache *Cache, ctx context.Context) (map[string]map[string]map[string]string, error) {

	users.lock.Lock()
	defer users.lock.Unlock()
	if users.resources != nil &&
		time.Now().Before(users.updatedAt.Add(time.Duration(config.Cfg.UserCacheTTL)*time.Millisecond)) {
		klog.V(5).Info("Using user's namespaced resources from cache.")
		return users.resources, users.err

	}

	resourceMap := make(map[string]map[string]map[string]string)
	klog.V(5).Info("Getting namespaced resources from ")
	resources, err := listResources()
	if err != nil {
		klog.Errorf("Error getting resources", err)
	}
	for _, resources := range resources {
		resourceMap["namespace"] = make(map[string]map[string]string)
		resourceMap["namespace"]["apigroupd"] = make(map[string]string)
		resourceMap["namespace"]["apigroupd"]["kinds"] = resources.Kind

	}
	fmt.Println("Resources:", resourceMap)

	return users.resources, users.err
}

func listResources() ([]metav1.APIResource, error) {
	// allresources := []*metav1.APIGroupList{}
	supportedNamepacedResources := []metav1.APIResource{}
	// supportedClusterScopedResources := []metav1.APIResource{}//removing since we are getting from cluster scoped from querying database

	// get all resources (namespcaed + cluster scopd) on cluster using kubeclient created for user:
	apiResources, err := config.KubeClient().ServerPreferredResources()

	if err != nil && apiResources == nil { // only return if the list is empty
		return nil, err
	} else if err != nil {
		klog.Warning("ServerPreferredResources could not list all available resources: ", err)
	}
	for _, apiList := range apiResources {

		for _, apiResource := range apiList.APIResources {
			for _, verb := range apiResource.Verbs {
				if verb == "list" {

					//get all resources that have list verb:
					if apiResource.Namespaced {
						supportedNamepacedResources = append(supportedNamepacedResources, apiResource)
					}

				}
			}
		}
	}
	return supportedNamepacedResources, err

}
