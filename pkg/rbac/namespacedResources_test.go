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
func mockNamespaceCache() Cache {

	return Cache{
		users:            map[string]*userData{},
		kubeClient:       fake.NewSimpleClientset(),
		corev1Client:     fake.NewSimpleClientset().CoreV1(),
		resConfig:        nil,
		tokenReviews:     map[string]*tokenReviewCache{},
		tokenReviewsLock: sync.Mutex{},
	}
}

func Test_getNamespaces_emptyCache(t *testing.T) {
	mock_cache := mockNamespaceCache()

	mock_cache.tokenReviews["123456"] = &tokenReviewCache{
		tokenReview: &authv1.TokenReview{
			Status: authv1.TokenReviewStatus{
				User: authv1.UserInfo{
					UID: "unique-user-id",
				},
			},
		},
	}

	ctx := context.Background()
	result, err := mock_cache.GetUserData(ctx, "123456")

	if len(result.namespaces) == 0 {
		t.Error("Resources not in cache.")
	}
	if err != nil {
		t.Error("Unexpected error while obtaining namespaces.", err)
	}

}

func Test_getNamespaces_usingCache(t *testing.T) {
	mock_cache := mockNamespaceCache()

	mock_cache.tokenReviews["123456"] = &tokenReviewCache{
		tokenReview: &authv1.TokenReview{
			Status: authv1.TokenReviewStatus{
				User: authv1.UserInfo{
					UID: "unique-user-id",
				},
			},
		},
	}

	var namespaces []string
	namespaces = append(namespaces, "open-cluster-management")
	namespaces = append(namespaces, "apps")
	mock_cache.users["123456"] = &userData{
		err:        nil,
		updatedAt:  time.Now(),
		namespaces: namespaces,
	}

	ctx := context.Background()

	result, err := mock_cache.GetUserData(ctx, "123456")

	if len(result.namespaces) == 0 {
		t.Error("Resources not in cache.")
	}
	if err != nil {
		t.Error("Unexpected error while obtaining namespaces.", err)
	}
	// Verify that cache was updated within the last 1 millisecond.
	if mock_cache.users["123456"].updatedAt.Before(time.Now().Add(time.Duration(-1) * time.Millisecond)) {
		t.Error("Expected cache.users.updatedAt to be less than 1 millisecond ago.")
	}

}

func Test_getNamespaces_expiredCache(t *testing.T) {
	mock_cache := mockNamespaceCache()

	mock_cache.tokenReviews["123456"] = &tokenReviewCache{
		tokenReview: &authv1.TokenReview{
			Status: authv1.TokenReviewStatus{
				User: authv1.UserInfo{
					UID: "unique-user-id",
				},
			},
		},
	}

	var namespaces []string
	namespaces = append(namespaces, "open-cluster-management")
	namespaces = append(namespaces, "apps")
	mock_cache.users["123456"] = &userData{
		err:        nil,
		namespaces: namespaces,
		updatedAt:  time.Now().Add(time.Duration(-5) * time.Minute)}

	ctx := context.Background()

	result, err := mock_cache.GetUserData(ctx, "123456")

	if len(result.namespaces) == 0 {
		t.Error("Resources not in cache.")
	}
	if err != nil {
		t.Error("Unexpected error while obtaining namespaces.", err)
	}

}
