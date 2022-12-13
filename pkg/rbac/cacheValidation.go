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

const RBAC_AUTHZ_K8S_IO = "rbac.authorization.k8s.io"

// Holds the objects needed to watch a kubernetes resource and trigger an action when a change is detected.
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
		onAdd:         c.namespaceAdded,
		onModify:      nil, // Ignoring MODIFY because it doesn't affect the cached data.
		onDelete:      c.namespaceDeleted,
	}
	go watchNamespaces.start(ctx)

	// Watch ManagedClusterAddon

	// Watch ROLES

	// Watch CLUSTERROLES

	// Watch ROLEBINDINGS

	// Watch CLUSTERRROLEBINDINGS

	// Watch GROUPS

	// Watch CRDS
}

// Start watching for changes to a resource and trigger the action to update the cache.
func (w watchResource) start(ctx context.Context) {
	for {
		watch, watchError := w.dynamicClient.Resource(w.gvr).Watch(ctx, metav1.ListOptions{})
		if watchError != nil {
			klog.Warningf("Error watching %s, waiting 5 seconds before retry. Error: %s", w.gvr.String(), watchError)
			time.Sleep(5 * time.Second) // Wait before retrying.
			continue
		}

		defer watch.Stop()
		klog.V(2).Infof("Watching resource: %s", w.gvr.String())

		for {
			select {
			case <-ctx.Done():
				klog.V(2).Info("Stopped watching resource. ", w.gvr.String())
				watch.Stop()
				return

			case event := <-watch.ResultChan(): // Read events from the watch channel.
				klog.V(6).Infof("Event: %s \tResource: %s  ", event.Type, w.gvr.String())
				o, err := runtime.UnstructuredConverter.ToUnstructured(runtime.DefaultUnstructuredConverter, &event.Object)
				if err != nil {
					klog.Warningf("Error converting %s event.Object to unstructured.Unstructured. Error: %s",
						w.gvr.Resource, err)
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
					klog.V(2).Infof("Unexpected event, waiting 5 seconds and restarting watch for %s", w.gvr.String())
					watch.Stop()
					time.Sleep(5 * time.Second)
				}
			}
		}
	}
}

func (c *Cache) clearUserData(obj *unstructured.Unstructured) {
	if c.pendingInvalidation {
		klog.V(5).Info("There's a pending request to clear the User Data cache.")
		return
	}
	c.pendingInvalidation = true
	klog.Info("Invalidating UserData cache. Waiting 5 seconds to 'debounce' or avoid triggering too many requests.")

	go func() {
		time.Sleep(5 * time.Second)

		c.usersLock.Lock()
		defer c.usersLock.Unlock()
		for _, userCache := range c.users {
			userCache.clustersCache.updatedAt = time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)
			userCache.csrCache.updatedAt = time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)
			userCache.nsrCache.updatedAt = time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)
		}
		c.pendingInvalidation = false
		klog.Info("Done invalidating the UserData cache.")
	}()
}

// Update the cache when a namespace is ADDED.
func (c *Cache) namespaceAdded(obj *unstructured.Unstructured) {
	c.shared.nsCache.lock.Lock()
	defer c.shared.nsCache.lock.Unlock()
	c.shared.namespaces = append(c.shared.namespaces, obj.GetName())
	c.shared.nsCache.updatedAt = time.Now()

	// Invalidate the ManagedClusters cache.
	c.shared.mcCache.updatedAt = time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)

	// Invalidate DisabledClusters cache.
	c.shared.dcCache.updatedAt = time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)

	// Invalidate UserData cache.
	c.clearUserData(obj)
}

// Update the cache when a namespace is DELETED.
func (c *Cache) namespaceDeleted(obj *unstructured.Unstructured) {
	c.shared.nsCache.lock.Lock()
	defer c.shared.nsCache.lock.Unlock()
	ns := obj.GetName()
	newNsamespaces := make([]string, 0)
	for _, n := range c.shared.namespaces {
		if n != ns {
			newNsamespaces = append(newNsamespaces, n)
		}
	}
	c.shared.namespaces = newNsamespaces
	c.shared.nsCache.updatedAt = time.Now()

	// Delete from ManagedClusters
	c.shared.mcCache.lock.Lock()
	defer c.shared.mcCache.lock.Unlock()
	delete(c.shared.managedClusters, ns)
	c.shared.mcCache.updatedAt = time.Now()

	// Delete from DisabledClusters
	c.shared.dcCache.lock.Lock()
	defer c.shared.dcCache.lock.Unlock()
	delete(c.shared.disabledClusters, ns)
	c.shared.dcCache.updatedAt = time.Now()

	// Delete from UserData caches
	c.usersLock.Lock()
	defer c.usersLock.Unlock()
	for _, userCache := range c.users {
		delete(userCache.UserData.NsResources, ns)
		delete(userCache.UserData.ManagedClusters, ns)
	}
}
