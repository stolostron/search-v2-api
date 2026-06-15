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
	mock_cache.tokenReviews[hashToken("1234567890")] = &tokenReviewCache{
		meta: cacheMetadata{updatedAt: time.Now()},
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
	mock_cache.tokenReviews[hashToken("1234567890-expired")] = &tokenReviewCache{
		authClient: fake.NewSimpleClientset().AuthenticationV1(),
		meta:       cacheMetadata{updatedAt: time.Now().Add(time.Duration(-5) * time.Minute)},
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
	if mock_cache.tokenReviews[hashToken("1234567890-expired")].meta.updatedAt.Before(time.Now().Add(time.Duration(-1) * time.Millisecond)) {
		t.Error("Expected the cached TokenReview to be updated within the last millisecond.")
	}

}

// Test_hashToken_keyIsHashed asserts that GetTokenReview stores entries under the
// SHA-256 hash of the token, not the raw token string.
func Test_hashToken_keyIsHashed(t *testing.T) {
	mock_cache := newMockCache()
	token := "super-secret-bearer-token"

	// Trigger a TokenReview — the fake client returns an unauthenticated result,
	// but a cache entry is still created.
	mock_cache.GetTokenReview(context.TODO(), token)

	if _, rawPresent := mock_cache.tokenReviews[token]; rawPresent {
		t.Error("raw token must not be stored as a cache key")
	}
	if _, hashedPresent := mock_cache.tokenReviews[hashToken(token)]; !hashedPresent {
		t.Error("SHA-256 hash of token must be used as the cache key")
	}
}
