// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"sync"

	v1 "k8s.io/client-go/kubernetes/typed/authentication/v1"
)

// Cache used to optimize requests to the Kubernetes API server.
type Cache struct {
	authClient          v1.AuthenticationV1Interface // This allows tests to replace with mock client.
	tokenReviews        map[string]*tokenReviewResult
	tokenReviewsPending map[string][]chan *tokenReviewResult
	tokenReviewsLock    sync.Mutex
}

// Initialize the cache as a singleton instance.
var cache = Cache{
	tokenReviews:        map[string]*tokenReviewResult{},
	tokenReviewsPending: map[string][]chan *tokenReviewResult{},
	tokenReviewsLock:    sync.Mutex{},
}
