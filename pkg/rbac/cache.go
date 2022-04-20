//
// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"time"

	klog "k8s.io/klog/v2"
)

type RbacCache struct {
	activeUsers map[string]UserRbac

	// Common data. These are the same for all users.
	namespaces   []string
	apiResources []interface{}

	lastValidation time.Time
}

// Builds an RbacCache object and tracks validation.
func (c *RbacCache) Initialize() RbacCache {

	return RbacCache{
		activeUsers:    make(map[string]UserRbac, 0),
		lastValidation: time.Now(),
	}
}

// Request the latest UserRbac.
func (c *RbacCache) GetUserRbac(token string) UserRbac {
	// Check for cached data.
	user, ok := c.activeUsers[token]
	if !ok {
		klog.Info("Token did not exist in cache. Building rules.")
	} else {
		if user.needUpdate {
			klog.Info("Found outdated cached data. Updating it.")
		}
	}

	user.lastActive = time.Now()
	return user
}
