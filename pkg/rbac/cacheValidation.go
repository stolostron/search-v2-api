// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

var (
	retryDelay = time.Duration(5) * time.Second
)

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

	// Watch ManagedClusters
	watchManagedClusters := watchResource{
		dynamicClient: c.shared.dynamicClient,
		gvr: schema.GroupVersionResource{
			Resource: "managedclusters",
			Group:    "cluster.open-cluster-management.io",
			Version:  "v1"},
		onAdd:    c.managedClusterAdded,
		onModify: nil, // Ignoring MODIFY because it doesn't affect the cached data.
		onDelete: c.managedClusterDeleted,
	}
	go watchManagedClusters.start(ctx)

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
			time.Sleep(retryDelay) // Wait before retrying.
			continue
		}

		defer watch.Stop()
		klog.V(2).Infof("Watching resource: %s", w.gvr.String())

		for {
			breakLoop := false
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
					time.Sleep(retryDelay)
					breakLoop = true
				}
			}
			if breakLoop {
				klog.Warningf("Restarting watch for %s", w.gvr.String())
				break
			}
		}
	}
}

// Update the cache when a namespace is ADDED.
func (c *Cache) namespaceAdded(obj *unstructured.Unstructured) {
	// Add namespace to shared cache.
	c.shared.nsCache.lock.Lock()
	c.shared.namespaces = append(c.shared.namespaces, obj.GetName())
	c.shared.nsCache.updatedAt = time.Now()
	c.shared.nsCache.lock.Unlock()

	// Add the namespace to each user in UserData cache.
	c.usersLock.Lock()
	defer c.usersLock.Unlock()
	wg := sync.WaitGroup{}
	lock := sync.Mutex{}
	for _, userCache := range c.users {
		wg.Add(1)
		go func(userCache *UserDataCache) { // All users updated asynchronously
			defer wg.Done()
			userCache.getSSRRforNamespace(context.TODO(), c, obj.GetName(), &lock)
		}(userCache)
	}
	wg.Wait() // Wait until all users have been updated.

	// Note: The ManagedCluster and DisabledClusters cache will get updated
	// when we receive the ManagedCluster ADD event.
}

// Update the cache when a namespace is DELETED.
func (c *Cache) namespaceDeleted(obj *unstructured.Unstructured) {
	// Delete from Namespaces shared cache.
	c.shared.nsCache.lock.Lock()
	ns := obj.GetName()
	newNamespaces := make([]string, 0)
	for _, n := range c.shared.namespaces {
		if n != ns {
			newNamespaces = append(newNamespaces, n)
		}
	}
	c.shared.namespaces = newNamespaces
	c.shared.nsCache.updatedAt = time.Now()
	c.shared.nsCache.lock.Unlock()

	// Delete from UserData cache
	c.usersLock.Lock()
	defer c.usersLock.Unlock()
	for _, userCache := range c.users {
		delete(userCache.UserData.NsResources, ns)     //nolint:staticcheck // "could remove embedded field 'UserData' from selector"
		delete(userCache.UserData.ManagedClusters, ns) //nolint:staticcheck // "could remove embedded field 'UserData' from selector"
	}
}

func (c *Cache) managedClusterAdded(obj *unstructured.Unstructured) {
	// Addd Managed Cluster to shared cache.
	c.shared.mcCache.lock.Lock()
	c.shared.managedClusters[obj.GetName()] = struct{}{}
	c.shared.mcCache.updatedAt = time.Now()
	c.shared.mcCache.lock.Unlock()

	// Update UserData cache for users with access to the managed cluster.
	c.usersLock.Lock()
	defer c.usersLock.Unlock()
	wg := sync.WaitGroup{}
	lock := sync.Mutex{}
	for _, userCache := range c.users {
		wg.Add(1)
		go func(userCache *UserDataCache) { // All users updated asynchronously
			defer wg.Done()
			// Refresh the SSRR, this will add the Managed cluster if user has access.
			userCache.getSSRRforNamespace(context.TODO(), c, obj.GetName(), &lock)
		}(userCache)
	}
	wg.Wait() // Wait until all users have been updated.
}

func (c *Cache) managedClusterDeleted(obj *unstructured.Unstructured) {
	// Delete ManagedCluster from shared cache
	c.shared.mcCache.lock.Lock()
	delete(c.shared.managedClusters, obj.GetName())
	c.shared.mcCache.updatedAt = time.Now()
	c.shared.mcCache.lock.Unlock()

	// Delete ManagedCluster from UserData cache
	c.usersLock.Lock()
	defer c.usersLock.Unlock()
	for _, userCache := range c.users {
		delete(userCache.UserData.ManagedClusters, obj.GetName()) //nolint:staticcheck // "could remove embedded field 'UserData' from selector"
	}

	// Delete from DisabledClusters shared cache
	c.shared.dcCache.lock.Lock()
	defer c.shared.dcCache.lock.Unlock()
	delete(c.shared.disabledClusters, obj.GetName())
	c.shared.dcCache.updatedAt = time.Now()
}
