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
	tokenReviews     map[string]*tokenReviewCache //Key:ClientToken
	tokenReviewsLock sync.Mutex
	shared           SharedData
	users            map[string]*userData // UID:{userdata} UID comes from tokenreview
	usersLock        sync.Mutex

	// Clients to external APIs.
	// Defining these here allow the tests to replace with a mock client.
	authnClient   authnv1.AuthenticationV1Interface
	corev1Client  corev1.CoreV1Interface
	pool          pgxpoolmock.PgxPool // Database client
	restConfig    *rest.Config
	dynamicClient dynamic.Interface
}

// Initialize the cache as a singleton instance.
var cacheInst = Cache{
	tokenReviews:     map[string]*tokenReviewCache{},
	tokenReviewsLock: sync.Mutex{},
	usersLock:        sync.Mutex{},
	shared:           SharedData{},
	users:            map[string]*userData{},
	restConfig:       config.GetClientConfig(),
	pool:             db.GetConnection(),
	corev1Client:     config.GetCoreClient(),
	dynamicClient:    config.GetDynamicClient(),
}
