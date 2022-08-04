package cache

import (
	"context"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// type CacheOptions struct {
// 	lazyInit      bool
// 	resolveData   func(context.Context) (error, interface{})
// 	shouldRefresh func(context.Context) bool
// 	ttl           time.Duration
// }

type CachedData struct {
	ctx context.Context

	// Configuration options
	lazyInit      bool
	resolveData   func(context.Context) (error, interface{})
	shouldRefresh func(context.Context) bool
	ttl           time.Duration

	// Internal fields used to manage the cache.
	data      interface{}
	lock      sync.Mutex
	err       error
	updatedAt time.Time
}

func NewCachedData(ctx context.Context) *CachedData {
	cd := &CachedData{
		ctx: ctx,
		// resolveData:   opts.resolveData,
		// shouldRefresh: opts.shouldRefresh,
		// ttl:           opts.ttl,
	}

	// Initialize the cache.
	// if !opts.lazyInit {
	go cd.doRefresh()
	// }

	return cd
}

// Access the data.
func (c *CachedData) Data() interface{} {
	c.lock.Lock()
	defer c.lock.Unlock()

	// Validate and update if needed.
	if !c.isValid() {
		c.doRefresh()
	} else {
		klog.Info("Using cached data.")
	}

	return c.data
}

func (c *CachedData) isValid() bool {
	if time.Now().After(c.updatedAt.Add(c.ttl)) {
		klog.V(2).Infof("Cache expired or never updated. UpdatedAt %s", c.updatedAt)
		return false
	} else if c.err != nil {
		klog.V(2).Infof("Cache has error response. Atempting to update. err %s", c.err)
		return false
	}
	return true
}

func (c *CachedData) doRefresh() {
	c.err, c.data = c.resolveData(c.ctx)
	c.updatedAt = time.Now()
}
