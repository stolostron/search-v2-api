package rbac

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	authv1 "k8s.io/api/authentication/v1"
	fake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

// Initialize cache object to use tests.
func mockNamespaceCache() Cache {

	return Cache{
		users:            map[string]*userData{},
		shared:           SharedData{},
		kubeClient:       fake.NewSimpleClientset(),
		restConfig:       &rest.Config{},
		tokenReviews:     map[string]*tokenReviewCache{},
		tokenReviewsLock: sync.Mutex{},
		authzClient:      fake.NewSimpleClientset().AuthorizationV1(),
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
	var namespaces []string
	mock_cache.shared.namespaces = append(namespaces, "open-cluster-management", "apps")

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

	mock_cache.users["123456"] = &userData{
		err:       nil,
		updatedAt: time.Now(),
	}
	var namespaces []string
	mock_cache.shared.namespaces = append(namespaces, "open-cluster-management", "apps")

	ctx := context.Background()

	fmt.Println("in mock cache", mock_cache.users["123456"].namespaces)

	result, err := mock_cache.GetUserData(ctx, "123456")

	if len(result.namespaces) == 0 {
		t.Error("Resources not in cache.")
	}
	if err != nil {
		t.Error("Unexpected error while obtaining namespaces.", err)
	}

	if mock_cache.users["123456"].updatedAt.After(time.Now().Add(time.Duration(-1) * time.Millisecond)) {
		t.Error("Expected the cache.users.updatedAt to be less than 2 millisecond ago.")
	}

}

func Test_getNamespaces_expiredCache(t *testing.T) {
	mock_cache := mockNamespaceCache()

	mock_cache.tokenReviews["123456-expired"] = &tokenReviewCache{
		tokenReview: &authv1.TokenReview{
			Status: authv1.TokenReviewStatus{
				User: authv1.UserInfo{
					UID: "unique-user-id",
				},
			},
		},
	}

	mock_cache.users["123456-expired"] = &userData{
		err:       nil,
		updatedAt: time.Now().Add(time.Duration(-5) * time.Minute)}

	var namespaces []string
	mock_cache.shared.namespaces = append(namespaces, "open-cluster-management", "apps")

	ctx := context.Background()

	result, err := mock_cache.GetUserData(ctx, "123456-expired")

	if len(result.namespaces) == 0 {
		t.Error("Resources not in cache.")
	}
	if err != nil {
		t.Error("Unexpected error while obtaining namespaces.", err)
	}
	// Verify that cache was updated within the last 2 millisecond.
	if mock_cache.users["123456-expired"].updatedAt.After(time.Now().Add(time.Duration(-1) * time.Millisecond)) {
		t.Error("Expected the cache.users.updatedAt to be less than 2 millisecond ago.")
	}

}
