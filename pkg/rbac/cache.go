// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"sync"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/stolostron/search-v2-api/pkg/config"
	db "github.com/stolostron/search-v2-api/pkg/database"
	authnv1 "k8s.io/client-go/kubernetes/typed/authentication/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// Cache helps optimize requests to external APIs (Kubernetes and Database)
type Cache struct {
	shared           SharedData
	tokenReviews     map[string]*tokenReviewCache //Key:ClientToken
	tokenReviewsLock sync.Mutex
	users            map[string]*UserDataCache // UID:{userdata} UID comes from tokenreview
	usersLock        sync.Mutex

	// Clients to external APIs.
	// Defining these here allow the tests to replace with a mock client.
	authnClient       authnv1.AuthenticationV1Interface
	pool              pgxpoolmock.PgxPool // Database client
	restConfig        *rest.Config
	dbConnInitialized bool
}

func (c *Cache) GetDbConnInitialized() bool {
	return c.dbConnInitialized
}

func (c *Cache) SetDbConnInitialized(initialized bool) {
	c.dbConnInitialized = initialized
}

// Initialize the cache as a singleton instance.
var cacheInst = Cache{
	tokenReviews:     map[string]*tokenReviewCache{},
	tokenReviewsLock: sync.Mutex{},
	usersLock:        sync.Mutex{},
	shared: SharedData{
		disabledClusters: map[string]struct{}{},
		managedClusters:  map[string]struct{}{},
		namespaces:       []string{},
		pool:             db.GetConnPool(context.TODO()),
		dynamicClient:    config.GetDynamicClient(),
	},
	users:      map[string]*UserDataCache{},
	restConfig: config.GetClientConfig(),
	pool:       db.GetConnPool(context.TODO()),
}

// Get a reference to the cache instance.
func GetCache() *Cache {
	ctx := context.TODO()
	// Workaround. Update cache connection with every request.
	// We need a better way to maintain this connection.
	if pool := db.GetConnPool(ctx); pool == nil {
		klog.Error("Unable to get a healthy database connection. Setting dbConnInitialized to false.")
		cacheInst.SetDbConnInitialized(false)
	} else {
		cacheInst.pool = pool
		cacheInst.shared.pool = pool
		klog.V(5).Info("Able to get a healthy database connection. Setting dbConnInitialized to true.")
		cacheInst.SetDbConnInitialized(true)
	}
	return &cacheInst
}

// IsHealthy checks if the RBAC cache is in a healthy state
func (c *Cache) IsHealthy() bool {
	// Check if database connection is initialized
	if !c.GetDbConnInitialized() {
		klog.V(3).Info("RBAC cache unhealthy: database connection not initialized")
		return false
	}

	// Check if we have a database pool
	if c.pool == nil {
		klog.V(3).Info("RBAC cache unhealthy: database pool is nil")
		return false
	}

	// we must acquire locks on mcCache and nsCache to ensure we read the correct up to cate cache objects as they are uninitialized and reinitialized every request
	c.shared.mcCache.lock.Lock()
	defer c.shared.mcCache.lock.Unlock()
	c.shared.nsCache.lock.Lock()
	defer c.shared.nsCache.lock.Unlock()
	hasData := c.shared.managedClusters != nil && c.shared.namespaces != nil

	if !hasData {
		klog.V(3).Info("RBAC cache unhealthy: shared data not initialized")
		return false
	}

	return true
}
