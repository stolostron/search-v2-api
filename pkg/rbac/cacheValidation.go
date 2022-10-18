// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

func StartCacheValidation(ctx context.Context) {
	for {
		GetCache().backgroundValidation(ctx)
		klog.Warning("Cache background validation stopped. Restarting process.")
		time.Sleep(1 * time.Second)
	}
}

func (c *Cache) backgroundValidation(ctx context.Context) {
	klog.Info("Starting cache background validation.")

	// Watch namespaces
	namespacesGVR := schema.GroupVersionResource{Resource: "namespaces", Group: "", Version: "v1"}
	go c.watchNamespaces(ctx, namespacesGVR)

	// Watch resources that could invalidate RBAC cache.
	rolesGVR := schema.GroupVersionResource{Resource: "roles", Group: "rbac.authorization.k8s.io", Version: "v1"}
	go c.watch(ctx, rolesGVR)

	clusterRolesGVR := schema.GroupVersionResource{Resource: "clusterroles", Group: "rbac.authorization.k8s.io", Version: "v1"}
	go c.watch(ctx, clusterRolesGVR)

	roleBindingsGVR := schema.GroupVersionResource{Resource: "rolebindings", Group: "rbac.authorization.k8s.io", Version: "v1"}
	go c.watch(ctx, roleBindingsGVR)

	// clusterRoleBindingsGVR := schema.GroupVersionResource{Resource: "clusterrolebindings", Group: "rbac.authorization.k8s.io", Version: "v1"}
	// c.watch(ctx, clusterRoleBindingsGVR)

	groupsGVR := schema.GroupVersionResource{Resource: "groups", Group: "user.openshift.io", Version: "v1"}
	go c.watch(ctx, groupsGVR)

	crdsGVR := schema.GroupVersionResource{Resource: "customresourcedefinitions", Group: "apiextensions.k8s.io", Version: "v1"}
	c.watch(ctx, crdsGVR)

}

// Watch resource and invalidate RBAC cache if anything changes.
func (c *Cache) watch(ctx context.Context, gvr schema.GroupVersionResource) {
	watch, watchError := c.shared.dynamicClient.Resource(gvr).Watch(ctx, metav1.ListOptions{})
	if watchError != nil {
		klog.Warningf("Error watching %s.  Error: %s", gvr.String(), watchError)
		return
	}
	defer watch.Stop()

	klog.V(3).Infof("Watching\t%s", gvr.String())

	for {
		select {
		case <-ctx.Done():
			klog.V(2).Info("Informer watch() was stopped. ", gvr.String())
			return

		case event := <-watch.ResultChan(): // Read events from the watch channel.
			// klog.Infof("Event: %s \tResource: %s  ", event.Type, gvr.String())
			switch event.Type {
			case "ADDED", "DELETED", "MODIFIED":
				c.invalidateCache()

			default:
				klog.V(2).Infof("Received unexpected event. Ending listAndWatch() for %s", gvr.String())
				return
			}
		}
	}
}

var cacheInvalidationPending bool

func (c *Cache) invalidateCache() {
	if cacheInvalidationPending {
		// klog.Info("There's a pending cache invalidation request.")
		return
	}
	cacheInvalidationPending = true
	klog.Info("Invalidating user data cache. Waiting 5 seconds to 'debounce' or avoid multiple invalidation requests.")

	go func() {
		time.Sleep(5 * time.Second)

		c.usersLock.Lock()
		defer c.usersLock.Unlock()
		for _, userCache := range c.users {
			userCache.clustersUpdatedAt = time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)
			userCache.csrUpdatedAt = time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)
			userCache.nsrUpdatedAt = time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)
		}
		cacheInvalidationPending = false
		klog.Info("Invalidated the user data cache.")
	}()
}
