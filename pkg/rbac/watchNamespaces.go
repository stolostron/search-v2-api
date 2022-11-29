// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Update the cache when a namespace is ADDED.
func (c *Cache) addNamespace(obj *unstructured.Unstructured) {
	c.shared.nsLock.Lock()
	defer c.shared.nsLock.Unlock()
	c.shared.namespaces = append(c.shared.namespaces, obj.GetName())
	c.shared.nsUpdatedAt = time.Now()

	// Invalidate the ManagedClusters cache.
	c.shared.mcUpdatedAt = time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)

	// Invalidate DisabledClusters cache.
	c.shared.dcUpdatedAt = time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)

	// Invalidate UserDataCache.
	c.usersLock.Lock()
	defer c.usersLock.Unlock()
	for _, userCache := range c.users {
		userCache.clustersUpdatedAt = time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)
		userCache.csrUpdatedAt = time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)
		userCache.nsrUpdatedAt = time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)
	}
}

// Update the cache when a namespace is DELETED.
func (c *Cache) deleteNamespace(obj *unstructured.Unstructured) {
	c.shared.nsLock.Lock()
	defer c.shared.nsLock.Unlock()
	ns := obj.GetName()
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
