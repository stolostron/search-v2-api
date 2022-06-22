package rbac

import (
	"context"
	"sync"
	"testing"
	"time"

	fake "k8s.io/client-go/kubernetes/fake"
	// fake "k8s.io/client-go/kubernetes/fake"
)

// Initialize cache object to use tests.
func mockNamespaceCache() Cache {

	return Cache{
		users:            map[string]*userData{},
		authClient:       fake.NewSimpleClientset().AuthenticationV1(),
		tokenReviews:     map[string]*tokenReviewCache{},
		tokenReviewsLock: sync.Mutex{},
		// coreClient: fake.NewSimpleClientset(),
	}
}

//test for empty cache
//test with cache

func Test_getNamespaces_emptyCache(t *testing.T) {
	mock_cache := mockNamespaceCache()
	ctx := context.Background()
	result, err := mock_cache.NamespacedResources(ctx, "123456")

	if len(result.namespaces) == 0 {
		t.Error("Resources not in cache.")
	}
	if err != nil {
		t.Error("Unexpected error while obtaining namespaces.", err)
	}
	// Verify that cache was updated within the last 1 millisecond.
	if mock_cache.users["123456"].updatedAt.Before(time.Now().Add(time.Duration(-1) * time.Millisecond)) {
		t.Error("Expected cache.shared.updatedAt to be less than 1 millisecond ago.")
	}

}

func Test_getNamespaces_usingCache(t *testing.T) {
	mock_cache := mockNamespaceCache()
	ctx := context.Background()
	var namespaces []string
	namespaces = append(namespaces, "open-cluster-management")
	namespaces = append(namespaces, "apps")
	mock_cache.users["123456"] = &userData{
		err:        nil,
		namespaces: namespaces,
		updatedAt:  time.Now()}

	result, err := mock_cache.NamespacedResources(ctx, "123456")

	if len(result.namespaces) == 0 {
		t.Error("Resources not in cache.")
	}
	if err != nil {
		t.Error("Unexpected error while obtaining namespaces.", err)
	}
	// Verify that cache was updated within the last 1 millisecond.
	if mock_cache.users["123456"].updatedAt.Before(time.Now().Add(time.Duration(-1) * time.Millisecond)) {
		t.Error("Expected cache.shared.updatedAt to be less than 1 millisecond ago.")
	}

}

func Test_getNamespaces_expiredCache(t *testing.T) {
	mock_cache := mockNamespaceCache()
	ctx := context.Background()
	var namespaces []string
	namespaces = append(namespaces, "open-cluster-management")
	namespaces = append(namespaces, "apps")
	mock_cache.users["123456"] = &userData{
		err:        nil,
		namespaces: namespaces,
		updatedAt:  time.Now().Add(time.Duration(-5) * time.Minute)}

	result, err := mock_cache.NamespacedResources(ctx, "123456")

	if len(result.namespaces) == 0 {
		t.Error("Resources not in cache.")
	}
	if err != nil {
		t.Error("Unexpected error while obtaining namespaces.", err)
	}
	// Verify that cache was updated within the last 1 millisecond.
	if mock_cache.users["123456"].updatedAt.Before(time.Now().Add(time.Duration(-1) * time.Millisecond)) {
		t.Error("Expected cache.users.updatedAt to be less than 2 milliseconds ago.")
	}

}
