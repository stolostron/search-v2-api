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

func (c *Cache) setDbConnInitialized(initialized bool) {
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
	klog.Info(("From getCache"))
	ctx := context.TODO()
	// Workaround. Update cache connection with every request.
	// We need a better way to maintain this connection.
	cacheInst.pool = db.GetConnPool(ctx)
	cacheInst.shared.pool = db.GetConnPool(ctx)

	if db.GetConnPool(ctx) == nil {
		klog.Error("Unable to get a healthy database connection. Setting dbConnInitialized to false.")
		cacheInst.setDbConnInitialized(false)
	} else {
		klog.V(2).Info("Able to get a healthy database connection. Setting dbConnInitialized to true.")
		cacheInst.setDbConnInitialized(true)
	}
	return &cacheInst
}
