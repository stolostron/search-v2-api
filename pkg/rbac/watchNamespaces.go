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

// Watch namespaces
func (c *Cache) watchNamespaces(ctx context.Context, gvr schema.GroupVersionResource) {
	// cache.corev1Client.Namespaces().List(ctx, metav1.ListOptions{})
	watch, watchError := c.dynamicClient.Resource(gvr).Watch(ctx, metav1.ListOptions{})
	if watchError != nil {
		klog.Warningf("Error watching %s.  Error: %s", gvr.String(), watchError)
		return
	}
	defer watch.Stop()

	klog.V(3).Infof("Watching\t[Group: %s \tKind: %s]", gvr.Group, gvr.Resource)

	for {
		select {
		case <-ctx.Done():
			klog.V(2).Info("Informer watch() was stopped. ", gvr.String())
			return

		case event := <-watch.ResultChan(): // Read events from the watch channel.
			//  Process ADDED or DELETED, events.
			o, error := runtime.UnstructuredConverter.ToUnstructured(runtime.DefaultUnstructuredConverter, &event.Object)
			if error != nil {
				klog.Warningf("Error converting %s event.Object to unstructured.Unstructured. Error: %s",
					gvr.Resource, error)
			}
			obj := &unstructured.Unstructured{Object: o}

			switch event.Type {
			case "ADDED":
				klog.V(3).Infof("Event: ADDED  Resource: %s  Name: %s", gvr.Resource, obj.GetName())
				c.addNamespace(obj.GetName())

			case "DELETED":
				klog.V(3).Infof("Event: DELETED  Resource: %s  Name: %s", gvr.Resource, obj.GetName())
				c.deleteNamespace(obj.GetName())

			case "MODIFIED":
				// Modified namespaces don't affect the cached data.
				break
			default:
				klog.V(2).Infof("Received unexpected event. Ending listAndWatch() for %s", gvr.String())
				return
			}
		}
	}
}

func (c *Cache) addNamespace(ns string) {
	c.shared.nsLock.Lock()
	defer c.shared.nsLock.Unlock()
	c.shared.namespaces = append(c.shared.namespaces, ns)
	c.shared.nsUpdatedAt = time.Now()

	// Invalidate the ManagedClusters cache. TODO: Update instead of invalidating.
	c.shared.mcUpdatedAt = time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)

	// Invalidate DisabledClusters cache. TODO: Update instead of invalidating.
	c.shared.dcUpdatedAt = time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)

	// Invalidate UserDataCache. TODO: Update instead of invalidating.
	c.usersLock.Lock()
	defer c.usersLock.Unlock()
	for _, userCache := range c.users {
		userCache.clustersUpdatedAt = time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)
		userCache.csrUpdatedAt = time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)
		userCache.nsrUpdatedAt = time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)
	}
}

func (c *Cache) deleteNamespace(ns string) {
	c.shared.nsLock.Lock()
	defer c.shared.nsLock.Unlock()
	newNsamespaces := make([]string, 0)
	for _, n := range c.shared.namespaces {
		if n != ns {
			newNsamespaces = append(newNsamespaces, n)
		}
	}
	c.shared.namespaces = newNsamespaces
	c.shared.nsUpdatedAt = time.Now()

	// Delete from ManagedClusters
	c.shared.mcLock.Lock()
	defer c.shared.mcLock.Unlock()
	delete(c.shared.managedClusters, ns)
	c.shared.mcUpdatedAt = time.Now()

	// Delete from DisabledClusters
	c.shared.dcLock.Lock()
	defer c.shared.dcLock.Unlock()
	delete(c.shared.disabledClusters, ns)
	c.shared.dcUpdatedAt = time.Now()

	// Delete from UserData caches
	c.usersLock.Lock()
	defer c.usersLock.Unlock()
	for _, userCache := range c.users {
		delete(userCache.userData.NsResources, ns)
		delete(userCache.userData.ManagedClusters, ns)
	}
}
