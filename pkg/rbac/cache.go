// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"sync"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/stolostron/search-v2-api/pkg/config"
	db "github.com/stolostron/search-v2-api/pkg/database"
	"k8s.io/client-go/dynamic"
	authnv1 "k8s.io/client-go/kubernetes/typed/authentication/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

// Cache used to minimize requests to external APIs (Kubernetes and Database)
type Cache struct {
	tokenReviews     map[string]*TokenReviewCache //Key:ClientToken
	tokenReviewsLock sync.Mutex
	shared           SharedData
	users            map[string]*UserDataCache // UID:{userdata} UID comes from tokenreview
	usersLock        sync.Mutex

	// Clients to external APIs.
	// Defining these here allow the tests to replace with a mock client.
	authnClient   authnv1.AuthenticationV1Interface
	corev1Client  corev1.CoreV1Interface
	Pool          pgxpoolmock.PgxPool // Database client
	RestConfig    *rest.Config
	dynamicClient dynamic.Interface
}

// Initialize the cache as a singleton instance.
var CacheInst = Cache{
	tokenReviews:     map[string]*TokenReviewCache{},
	tokenReviewsLock: sync.Mutex{},
	usersLock:        sync.Mutex{},
	shared:           SharedData{},
	users:            map[string]*UserDataCache{},
	RestConfig:       config.GetClientConfig(),
	Pool:             db.GetConnection(),
	corev1Client:     config.GetCoreClient(),
	dynamicClient:    config.GetDynamicClient(),
}

func (cache *Cache) SetTokenReviews(tokenReviews map[string]*TokenReviewCache) *Cache {
	cache.tokenReviews = tokenReviews
	return cache
}
