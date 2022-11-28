// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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

// Watch resources that could invalidate RBAC cache.
func (c *Cache) backgroundValidation(ctx context.Context) {
	klog.Info("Starting cache background validation.")

	// Watch NAMESPACES
	// Only ADD and DELETE affect the cached data, ignore MODIFY.
	namespacesGVR := schema.GroupVersionResource{Resource: "namespaces", Group: "", Version: "v1"}
	go c.watch(ctx, namespacesGVR, c.addNamespace, nil, c.deleteNamespace)

	// Watch ROLES
	rolesGVR := schema.GroupVersionResource{Resource: "roles", Group: "rbac.authorization.k8s.io", Version: "v1"}
	go c.watch(ctx, rolesGVR, c.forceExpiration, c.forceExpiration, c.forceExpiration)

	// Watch CLUSTERROLES
	clusterRolesGVR := schema.GroupVersionResource{Resource: "clusterroles", Group: "rbac.authorization.k8s.io", Version: "v1"}
	go c.watch(ctx, clusterRolesGVR, c.forceExpiration, c.forceExpiration, c.forceExpiration)

	// Watch ROLEBINDINGS
	roleBindingsGVR := schema.GroupVersionResource{Resource: "rolebindings", Group: "rbac.authorization.k8s.io", Version: "v1"}
	go c.watch(ctx, roleBindingsGVR, c.forceExpiration, skipEvent, c.forceExpiration)

	// Watch CLUSTERRROLEBINDINGS
	clusterRoleBindingsGVR := schema.GroupVersionResource{Resource: "clusterrolebindings", Group: "rbac.authorization.k8s.io", Version: "v1"}
	c.watch(ctx, clusterRoleBindingsGVR, c.forceExpiration, skipEvent, c.forceExpiration)

	// Watch GROUPS
	groupsGVR := schema.GroupVersionResource{Resource: "groups", Group: "user.openshift.io", Version: "v1"}
	go c.watch(ctx, groupsGVR, c.forceExpiration, c.forceExpiration, c.forceExpiration)

	// Watch CRDS
	// Only ADDED CRDs require a cache refresh. Deletions can wait for normal expiration.
	crdsGVR := schema.GroupVersionResource{Resource: "customresourcedefinitions", Group: "apiextensions.k8s.io", Version: "v1"}
	c.watch(ctx, crdsGVR, c.forceExpiration, nil, nil)
}

func skipEvent(obj *unstructured.Unstructured) {
	klog.Infof(">>> Ignoring event. KIND: %s  NAME: %s", obj.GetKind(), obj.GetName())
}

// Watch resource and invalidate RBAC cache if anything changes.
func (c *Cache) watch(ctx context.Context, gvr schema.GroupVersionResource, onAdd, onModify, onDelete func(*unstructured.Unstructured)) {
	watch, watchError := c.shared.dynamicClient.Resource(gvr).Watch(ctx, metav1.ListOptions{})
	if watchError != nil {
		klog.Warningf("Error watching resource %s. Error: %s", gvr.String(), watchError)
		return
	}
	defer watch.Stop()

	klog.V(3).Infof("Watching resource: %s", gvr.String())

	for {
		select {
		case <-ctx.Done():
			klog.V(2).Info("Stopped watching resource. ", gvr.String())
			return

		case event := <-watch.ResultChan(): // Read events from the watch channel.
			// klog.Infof("Event: %s \tResource: %s  ", event.Type, gvr.String())
			o, error := runtime.UnstructuredConverter.ToUnstructured(runtime.DefaultUnstructuredConverter, &event.Object)
			if error != nil {
				klog.Warningf("Error converting %s event.Object to unstructured.Unstructured. Error: %s",
					gvr.Resource, error)
			}
			obj := &unstructured.Unstructured{Object: o}

			switch event.Type {
			case "ADDED":
				if onAdd != nil {
					onAdd(obj)
				}
			case "MODIFIED":
				if onModify != nil {
					onModify(obj)
				}
			case "DELETED":
				if onDelete != nil {
					onDelete(obj)
				}

			default:
				klog.V(2).Infof("Received unexpected event. Ending listAndWatch() for %s", gvr.String())
				return
			}
		}
	}
}

var pendingExpiration bool

func (c *Cache) forceExpiration(obj *unstructured.Unstructured) {
	klog.V(9).Info("obj:", obj)
	if pendingExpiration {
		klog.V(9).Info("There's a pending request to force the cache to expire.")
		return
	}
	pendingExpiration = true
	klog.Info("Invalidating user data cache. Waiting 5 seconds to 'debounce' or avoid too many invalidation requests.")

	go func() {
		time.Sleep(5 * time.Second)

		c.usersLock.Lock()
		defer c.usersLock.Unlock()
		for _, userCache := range c.users {
			userCache.clustersUpdatedAt = time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)
			userCache.csrUpdatedAt = time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)
			userCache.nsrUpdatedAt = time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)
		}
		pendingExpiration = false
		klog.Info("Invalidated the user data cache.")
	}()
}
