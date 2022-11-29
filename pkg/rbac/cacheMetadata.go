// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"sync"
	"time"

	"github.com/stolostron/search-v2-api/pkg/config"
)

// Common fields to manage a cached data field.
type cacheMetadata struct {
	err       error         // Error while retrieving the data from external API.
	lock      sync.Mutex    // Locks the data field while requesting the latest data.
	updatedAt time.Time     // Time when the data field was last updated.
	ttl       time.Duration // Time-to-live, time duration for which this cache is valid.
}

// Checks if the cached data is valid or expired.
func (cacheMeta *cacheMetadata) isValid() bool {
	// Default TTL
	cacheTTL := time.Duration(config.Cfg.SharedCacheTTL) * time.Millisecond

	// Custom TTL
	if cacheMeta.ttl > 0 {
		cacheTTL = cacheMeta.ttl
	}

	// Error TTL. Errors are valid for a shorter period to allow fast recovery.
	if cacheMeta.err != nil {
		cacheTTL = time.Duration(1000) * time.Millisecond
	}
	return time.Now().Before(cacheMeta.updatedAt.Add(cacheTTL))
}
