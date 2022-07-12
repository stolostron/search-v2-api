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
func newMockCache() Cache {
	return Cache{
		// Use a fake Kubernetes authentication client.
		authnClient:      fake.NewSimpleClientset().AuthenticationV1(),
		tokenReviews:     map[string]*tokenReviewCache{},
		tokenReviewsLock: sync.Mutex{},
	}
}

// TokenReview with empty cache.
func Test_IsValidToken_emptyCache(t *testing.T) {
	// Initialize cache with empty state.
	mock_cache := newMockCache()

	// Execute function
	result, err := mock_cache.IsValidToken(context.TODO(), "1234567890")

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
	mock_cache := newMockCache()
	mock_cache.tokenReviews["1234567890"] = &tokenReviewCache{
		updatedAt: time.Now(),
		tokenReview: &authv1.TokenReview{
			Status: authv1.TokenReviewStatus{
				Authenticated: true,
			},
		},
	}

	// Execute function
	result, err := mock_cache.IsValidToken(context.TODO(), "1234567890")

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
	// Initialize cache and set state to TokenReview updated 5 minutes ago.
	mock_cache := newMockCache()
	mock_cache.tokenReviews["1234567890-expired"] = &tokenReviewCache{
		authClient: fake.NewSimpleClientset().AuthenticationV1(),
		updatedAt:  time.Now().Add(time.Duration(-5) * time.Minute),
		token:      "1234567890-expired",
		tokenReview: &authv1.TokenReview{
			Status: authv1.TokenReviewStatus{
				Authenticated: true,
			},
		},
	}

	// Execute function
	result, err := mock_cache.IsValidToken(context.TODO(), "1234567890-expired")

	// Validate results
	if result {
		t.Error("Expected token to be invalid.")
	}
	if err != nil {
		t.Error("Received unexpected error from IsValidToken()", err)
	}
	// Verify that cache was updated within the last 1 millisecond.
	// if mock_cache.tokenReviews["1234567890-expired"].updatedAt.Before(time.Now().Add(time.Duration(-1) * time.Millisecond)) {
	// 	t.Error("Expected the cached TokenReview to be updated within the last millisecond.")
	// }

}
