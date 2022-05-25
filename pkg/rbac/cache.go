// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"sync"
)

var cache = Cache{
	tokenReviews:        map[string]*tokenReviewResult{},
	tokenReviewsPending: map[string][]chan *tokenReviewResult{},
	tokenReviewsLock:    sync.Mutex{},
}

type Cache struct {
	tokenReviews        map[string]*tokenReviewResult
	tokenReviewsPending map[string][]chan *tokenReviewResult
	tokenReviewsLock    sync.Mutex
}
