// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"sync"
	"testing"
	"time"

	authv1 "k8s.io/api/authentication/v1"
	fake "k8s.io/client-go/kubernetes/fake"
)

// Initialize cache object to use tests.
func newCache() Cache {
	return Cache{
		// Use a fake Kubernetes authentication client.
		authClient:          fake.NewSimpleClientset().AuthenticationV1(),
		tokenReviews:        map[string]*tokenReviewResult{},
		tokenReviewsPending: map[string][]chan *tokenReviewResult{},
		tokenReviewsLock:    sync.Mutex{},
	}
}

// TokenReview with empty cache.
func Test_IsValidToken_emptyCache(t *testing.T) {
	// Initialize cache with empty state.
	cache := newCache()

	// Execute function
	result, err := cache.IsValidToken(context.TODO(), "1234567890")

	// Validate results
	if result {
		t.Error("Expected token to be invalid.")
	}
	if err != nil {
		t.Error("Received unexpected error from IsValidToken()", err)
	}
}

// TokenReview exists in cache
func Test_IsValidToken_usingCache(t *testing.T) {
	// Initialize cache and set state.
	cache := newCache()
	cache.tokenReviews["1234567890"] = &tokenReviewResult{
		updatedAt: time.Now(),
		tokenReview: &authv1.TokenReview{
			Status: authv1.TokenReviewStatus{
				Authenticated: true,
			},
		},
	}

	// Execute function
	result, err := cache.IsValidToken(context.TODO(), "1234567890")

	// Validate results
	if !result {
		t.Error("Expected token to be valid (using cached TokenReview).")
	}
	if err != nil {
		t.Error("Received unexpected error from IsValidToken()", err)
	}
}

// TokenReview in cache is older than 60 seconds.
func Test_IsValidToken_expiredCache(t *testing.T) {
	// Initialize cache and set state to TokenReview updated 2 minutes ago.
	cache := newCache()
	cache.tokenReviews["1234567890"] = &tokenReviewResult{
		updatedAt: time.Now().Add(time.Duration(-2) * time.Minute),
		tokenReview: &authv1.TokenReview{
			Status: authv1.TokenReviewStatus{
				Authenticated: true,
			},
		},
	}

	// Execute function
	result, err := cache.IsValidToken(context.TODO(), "1234567890")

	// Validate results
	if result {
		t.Error("Expected token to be invalid.")
	}
	if err != nil {
		t.Error("Received unexpected error from IsValidToken()", err)
	}
	// Verify that cache was updated within the last 1 millisecond.
	if cache.tokenReviews["1234567890"].updatedAt.Before(time.Now().Add(time.Duration(-1) * time.Millisecond)) {
		t.Error("Expected the cached TokenReview to be updated within the last millisecond.")
	}

}

// TokenReview pending request for same token.
func Test_IsValidToken_pendingRequest(t *testing.T) {
	// Initialize cache state with a pending TokenReview.
	cache := newCache()
	cache.tokenReviewsPending["1234567890-pending"] = []chan *tokenReviewResult{make(chan *tokenReviewResult)}

	// Execute function
	var testCH chan *tokenReviewResult
	cache.doTokenReview(context.TODO(), "1234567890-pending", testCH)

	// Validate result
	if len(cache.tokenReviewsPending["1234567890-pending"]) != 2 {
		t.Error("Expected channel to be added to pendingTokenReviews.")
	}
}
