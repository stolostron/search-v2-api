// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

type watchResource struct {
	dynamicClient dynamic.Interface
	gvr           schema.GroupVersionResource
	onAdd         func(*unstructured.Unstructured)
	onModify      func(*unstructured.Unstructured)
	onDelete      func(*unstructured.Unstructured)
}

// Watch resources that could invalidate the cache.
func (c *Cache) StartBackgroundValidation(ctx context.Context) {
	klog.Info("Starting cache background validation.")

	// Watch NAMESPACES
	watchNamespaces := watchResource{
		dynamicClient: c.shared.dynamicClient,
		gvr:           schema.GroupVersionResource{Resource: "namespaces", Group: "", Version: "v1"},
		onAdd:         c.addNamespace,
		onModify:      nil, // Ignoring MODIFY because it doesn't affect the cached data.
		onDelete:      c.deleteNamespace,
	}
	go watchNamespaces.start(ctx)

	// Watch ROLES
	watchRoles := watchResource{
		dynamicClient: c.shared.dynamicClient,
		gvr:           schema.GroupVersionResource{Resource: "roles", Group: "rbac.authorization.k8s.io", Version: "v1"},
		onAdd:         c.clearUserData,
		onModify:      c.clearUserData,
		onDelete:      c.clearUserData,
	}
	go watchRoles.start(ctx)

	// Watch CLUSTERROLES
	watchClusterRoles := watchResource{
		dynamicClient: c.shared.dynamicClient,
		gvr:           schema.GroupVersionResource{Resource: "clusterroles", Group: "rbac.authorization.k8s.io", Version: "v1"},
		onAdd:         c.clearUserData,
		onModify:      c.clearUserData,
		onDelete:      c.clearUserData,
	}
	go watchClusterRoles.start(ctx)

	// Watch ROLEBINDINGS
	watchRoleBindings := watchResource{
		dynamicClient: c.shared.dynamicClient,
		gvr:           schema.GroupVersionResource{Resource: "rolebindings", Group: "rbac.authorization.k8s.io", Version: "v1"},
		onAdd:         c.clearUserData,
		onModify:      nil, // FIXME: Skipping MODIFY because we are receiving too many events.
		onDelete:      c.clearUserData,
	}
	go watchRoleBindings.start(ctx)

	// Watch CLUSTERRROLEBINDINGS
	watchClusterRoleBindings := watchResource{
		dynamicClient: c.shared.dynamicClient,
		gvr:           schema.GroupVersionResource{Resource: "clusterrolebindings", Group: "rbac.authorization.k8s.io", Version: "v1"},
		onAdd:         c.clearUserData,
		onModify:      nil, // FIXME: Skipping MODIFY because we are receiving too many events.
		onDelete:      c.clearUserData,
	}
	go watchClusterRoleBindings.start(ctx)

	// Watch GROUPS
	watchGroups := watchResource{
		dynamicClient: c.shared.dynamicClient,
		gvr:           schema.GroupVersionResource{Resource: "groups", Group: "user.openshift.io", Version: "v1"},
		onAdd:         c.clearUserData,
		onModify:      c.clearUserData,
		onDelete:      c.clearUserData,
	}
	go watchGroups.start(ctx)

	// Watch CRDS
	watchCRDs := watchResource{
		dynamicClient: c.shared.dynamicClient,
		gvr:           schema.GroupVersionResource{Resource: "customresourcedefinitions", Group: "apiextensions.k8s.io", Version: "v1"},
		onAdd:         c.clearUserData,
		onModify:      c.clearUserData,
		onDelete:      nil, // Deletions can wait for normal expiration.
	}
	go watchCRDs.start(ctx)
}

// func skipEvent(obj *unstructured.Unstructured) {
// 	klog.Infof(">>> Ignoring event. KIND: %s  NAME: %s", obj.GetKind(), obj.GetName())
// }

// Start watching for configuration changes and invalidate RBAC cache.
func (w watchResource) start(ctx context.Context) {
	for {
		watch, watchError := w.dynamicClient.Resource(w.gvr).Watch(ctx, metav1.ListOptions{})
		if watchError != nil {
			klog.Warningf("Error watching resource %s. Error: %s", w.gvr.String(), watchError)
			time.Sleep(5 * time.Second)
			continue
		}

		defer watch.Stop()
		klog.V(2).Infof("Watching resource: %s", w.gvr.String())

		for {
			select {
			case <-ctx.Done():
				klog.V(2).Info("Stopped watching resource. ", w.gvr.String())
				return

			case event := <-watch.ResultChan(): // Read events from the watch channel.
				klog.V(6).Infof("Event: %s \tResource: %s  ", event.Type, w.gvr.String())
				o, error := runtime.UnstructuredConverter.ToUnstructured(runtime.DefaultUnstructuredConverter, &event.Object)
				if error != nil {
					klog.Warningf("Error converting %s event.Object to unstructured.Unstructured. Error: %s",
						w.gvr.Resource, error)
				}
				obj := &unstructured.Unstructured{Object: o}

				switch event.Type {
				case "ADDED":
					if w.onAdd != nil {
						w.onAdd(obj)
					}
				case "MODIFIED":
					if w.onModify != nil {
						w.onModify(obj)
					}
				case "DELETED":
					if w.onDelete != nil {
						w.onDelete(obj)
					}

				default:
					klog.V(2).Infof("Received unexpected event. Restarting watch for %s", w.gvr.String())
					watch.Stop()
				}
			}
		}
	}
}

var pendingInvalidation bool

func (c *Cache) clearUserData(obj *unstructured.Unstructured) {
	if pendingInvalidation {
		klog.V(9).Info("There's a pending request to force the cache to expire.")
		return
	}
	pendingInvalidation = true
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
		pendingInvalidation = false
		klog.Info("Done invalidating the user data cache.")
	}()
}
