// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stolostron/search-v2-api/pkg/config"
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
	mock_cache.tokenReviews["1234567890-expired"] = &tokenReviewCache{
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
	if mock_cache.tokenReviews["1234567890-expired"].meta.updatedAt.Before(time.Now().Add(time.Duration(-1) * time.Millisecond)) {
		t.Error("Expected the cached TokenReview to be updated within the last millisecond.")
	}

}

// Test_evictExpiredTokenReviews verifies that entries whose updatedAt is older
// than the AuthCacheTTL are removed from the map the next time GetTokenReview
// is called.
func Test_evictExpiredTokenReviews(t *testing.T) {
	mock_cache := newMockCache()

	// Insert one expired entry and one fresh entry.
	expiredToken := "expired-token"
	freshToken := "fresh-token"
	mock_cache.tokenReviews[expiredToken] = &tokenReviewCache{
		authClient: mock_cache.authnClient,
		token:      expiredToken,
		meta:       cacheMetadata{updatedAt: time.Now().Add(-10 * time.Minute)},
		tokenReview: &authv1.TokenReview{
			Status: authv1.TokenReviewStatus{Authenticated: true},
		},
	}
	mock_cache.tokenReviews[freshToken] = &tokenReviewCache{
		authClient: mock_cache.authnClient,
		token:      freshToken,
		meta:       cacheMetadata{updatedAt: time.Now()},
		tokenReview: &authv1.TokenReview{
			Status: authv1.TokenReviewStatus{Authenticated: true},
		},
	}

	// Trigger a GetTokenReview call for any token — this runs eviction.
	if _, err := mock_cache.GetTokenReview(context.TODO(), "trigger-eviction"); err != nil {
		t.Fatalf("unexpected error from GetTokenReview: %v", err)
	}

	if _, stillPresent := mock_cache.tokenReviews[expiredToken]; stillPresent {
		t.Error("Expected expired TokenReview entry to be evicted from the cache.")
	}
	if _, gone := mock_cache.tokenReviews[freshToken]; !gone {
		t.Error("Expected fresh TokenReview entry to remain in the cache.")
	}
}

// Test_tokenReviewCacheSizeCap verifies that the cache never exceeds
// config.Cfg.AuthCacheMaxSize entries.
func Test_tokenReviewCacheSizeCap(t *testing.T) {
	mock_cache := newMockCache()
	maxSize := config.Cfg.AuthCacheMaxSize

	// Pre-fill the cache to exactly the maximum with entries that have a
	// recent updatedAt so they are not expired during the test.
	for i := 0; i < maxSize; i++ {
		token := fmt.Sprintf("existing-token-%d", i)
		mock_cache.tokenReviews[token] = &tokenReviewCache{
			authClient: mock_cache.authnClient,
			token:      token,
			meta:       cacheMetadata{updatedAt: time.Now()},
			tokenReview: &authv1.TokenReview{
				Status: authv1.TokenReviewStatus{Authenticated: true},
			},
		}
	}

	// Adding one more token should not grow the map beyond the cap.
	if _, err := mock_cache.GetTokenReview(context.TODO(), "overflow-token"); err != nil {
		t.Fatalf("unexpected error from GetTokenReview: %v", err)
	}

	if got := len(mock_cache.tokenReviews); got > maxSize {
		t.Errorf("Expected tokenReviews cache size <= %d, got %d", maxSize, got)
	}
}
