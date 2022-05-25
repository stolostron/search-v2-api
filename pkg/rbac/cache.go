// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"sync"
)

// Cache used to optimize requests to the Kubernetes API server.
type Cache struct {
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
