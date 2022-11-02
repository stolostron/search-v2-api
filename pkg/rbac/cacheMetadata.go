// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"sync"
	"time"

	"github.com/stolostron/search-v2-api/pkg/config"
)

// Common fields to manage a cached data field.
type cacheMetadata struct {
	err       error      // Error while retrieving the data from external API.
	lock      sync.Mutex // Locks the data field while requesting the latest data.
	updatedAt time.Time  // Time when the data field was last updated.
}

func (cacheMeta *cacheMetadata) isValid() bool {
	if cacheMeta.err != nil {
		return false
	}
	// TODO handle different cache durations.
	cacheDuration := time.Duration(config.Cfg.SharedCacheTTL) * time.Millisecond
	if time.Now().Before(cacheMeta.updatedAt.Add(cacheDuration)) {
		return true
	}
	return false
}
