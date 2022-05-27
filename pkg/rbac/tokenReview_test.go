// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"sync"
	"testing"
	"time"

	authv1 "k8s.io/api/authentication/v1"
)

func Test_IsValidToken_emptyCache(t *testing.T) {
	cache := Cache{
		tokenReviews:        map[string]*tokenReviewResult{},
		tokenReviewsPending: map[string][]chan *tokenReviewResult{},
		tokenReviewsLock:    sync.Mutex{},
	}

	result, err := cache.IsValidToken(context.TODO(), "1234567890")

	if result { // TODO
		t.Error("Expected token to resolve to valid.")
	}
	if err != nil {
		t.Error("Received unexpected error from IsValidToken()", err)
	}
}

// TokenReview exists in cache.
func Test_IsValidToken_usingCache(t *testing.T) {
	cache := Cache{
		tokenReviews:        map[string]*tokenReviewResult{},
		tokenReviewsPending: map[string][]chan *tokenReviewResult{},
		tokenReviewsLock:    sync.Mutex{},
	}
	cache.tokenReviews["1234567890"] = &tokenReviewResult{
		updatedAt:   time.Now(),
		tokenReview: &authv1.TokenReview{},
	}

	result, err := cache.IsValidToken(context.TODO(), "1234567890")

	if result { // TODO
		t.Error("Expected token to resolve to valid.")
	}
	if err != nil {
		t.Error("Received unexpected error from IsValidToken()", err)
	}
}

// TokenReview in cache is older than 60 seconds.
func Test_IsValidToken_expiredCache(t *testing.T) {
	cache := Cache{
		tokenReviews:        map[string]*tokenReviewResult{},
		tokenReviewsPending: map[string][]chan *tokenReviewResult{},
		tokenReviewsLock:    sync.Mutex{},
	}
	cache.tokenReviews["1234567890"] = &tokenReviewResult{
		updatedAt:   time.Now().Add(time.Duration(-5) * time.Minute),
		tokenReview: &authv1.TokenReview{},
	}

	result, err := cache.IsValidToken(context.TODO(), "1234567890")

	if result {
		t.Error("Expected token to resolve to valid.")
	}
	if err != nil {
		t.Error("Received unexpected error from IsValidToken()", err)
	}
	// TODO: Verify that cache got updated.
	if cache.tokenRevews["1234567890"].updatedAt
}

// Pending TokenReview request.
func Test_IsValidToken_pendingRequest(t *testing.T) {
	cache := Cache{
		tokenReviews:        map[string]*tokenReviewResult{},
		tokenReviewsPending: map[string][]chan *tokenReviewResult{},
		tokenReviewsLock:    sync.Mutex{},
	}
	cache.tokenReviewsPending["1234567890-pending"] = []chan *tokenReviewResult{make(chan *tokenReviewResult)}

	var testCH chan *tokenReviewResult
	cache.doTokenReview(context.TODO(), "1234567890-pending", testCH)

	if len(cache.tokenReviewsPending["1234567890-pending"]) != 2 {
		t.Error("Expected channel to be added to pendingTokenReviews.")
	}
}
