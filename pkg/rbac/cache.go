// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"sync"
	"time"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/stolostron/search-v2-api/pkg/config"
	db "github.com/stolostron/search-v2-api/pkg/database"
	authnv1 "k8s.io/client-go/kubernetes/typed/authentication/v1"
	"k8s.io/client-go/rest"
)

// Cache helps optimize requests to external APIs (Kubernetes and Database)
type Cache struct {
	tokenReviews     map[string]*tokenReviewCache //Key:ClientToken
	tokenReviewsLock sync.Mutex
	shared           SharedData
	users            map[string]*UserDataCache // UID:{userdata} UID comes from tokenreview
	usersLock        sync.Mutex

	// Clients to external APIs.
	// Defining these here allow the tests to replace with a mock client.
	authnClient authnv1.AuthenticationV1Interface
	pool        pgxpoolmock.PgxPool // Database client
	restConfig  *rest.Config
}

// Common fields to manage a cached data field.
type cacheMetadata struct {
	err       error      // Error while retrieving the data from external API.
	lock      sync.Mutex // Locks the data field while requesting the latest data.
	updatedAt time.Time  // Time when the data field was last updated.
}

// Initialize the cache as a singleton instance.
var cacheInst = Cache{
	tokenReviews:     map[string]*tokenReviewCache{},
	tokenReviewsLock: sync.Mutex{},
	usersLock:        sync.Mutex{},
	shared: SharedData{
		pool:          db.GetConnection(),
		corev1Client:  config.GetCoreClient(),
		dynamicClient: config.GetDynamicClient(),
	},
	users:      map[string]*UserDataCache{},
	restConfig: config.GetClientConfig(),
	pool:       db.GetConnection(),
}

// Get a reference to the cache instance.
func GetCache() *Cache {
	// Workaround. Update cache connection with every request.
	// We need a better way to maintain this connection.
	cacheInst.pool = db.GetConnection()
	cacheInst.shared.pool = db.GetConnection()

	return &cacheInst
}
