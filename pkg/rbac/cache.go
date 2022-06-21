// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"sync"

	"github.com/driftprogramming/pgxpoolmock"
	db "github.com/stolostron/search-v2-api/pkg/database"
	v1 "k8s.io/client-go/kubernetes/typed/authentication/v1"
)

// Cache used to optimize requests to the Kubernetes API server.
type Cache struct {
	authClient       v1.AuthenticationV1Interface // This allows tests to replace with mock client.
	tokenReviews     map[string]*tokenReviewCache
	tokenReviewsLock sync.Mutex
	shared           clusterScopedResources
	users            map[string]*userData // UID:{userdata} UID comes from tokenreview
	pool             pgxpoolmock.PgxPool
}

// Initialize the cache as a singleton instance.
var cacheInst = Cache{
	tokenReviews:     map[string]*tokenReviewCache{},
	tokenReviewsLock: sync.Mutex{},
	shared:           clusterScopedResources{},
	users:            map[string]*userData{},
	pool:             db.GetConnection(),
}
