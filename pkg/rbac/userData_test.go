package rbac

import (
	"context"
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
	mock_cache.shared.namespaces = append(namespaces, "some-namespace")

	ctx := context.Background()
	result, err := mock_cache.GetUserData(ctx, "123456")

	if len(result.nsResources) > 0 {
		t.Error("Cache should be empty.")
	}
	if err != nil {
		t.Error("Unexpected error while obtaining namespaces.", err)
	}

}

func Test_getNamespaces_usingCache(t *testing.T) {
	var namespaces []string
	nsresources := make(map[string][]resource)

	mock_cache := mockNamespaceCache()

	//mock cache for token review to get user data:
	mock_cache.tokenReviews["123456"] = &tokenReviewCache{
		tokenReview: &authv1.TokenReview{
			Status: authv1.TokenReviewStatus{
				User: authv1.UserInfo{
					UID: "unique-user-id",
				},
			},
		},
	}

	//mock cache for cluster-scoped resouces to get all namespaces:
	mock_cache.shared.namespaces = append(namespaces, "some-namespace")
	//mock cache for namespaced-resources:
	nsresources["some-namespace"] = append(nsresources["some-namespace"],
		resource{apigroup: "some-apigroup", kind: "some-kind"})

	mock_cache.users["unique-user-id"] = &userData{
		nsResources:  nsresources,
		nsrUpdatedAt: time.Now(),
	}

	result, err := mock_cache.GetUserData(context.Background(), "123456")

	if len(result.nsResources) == 0 {
		t.Error("Resources not in cache.")
	}
	if err != nil {
		t.Error("Unexpected error while obtaining namespaces.", err)
	}

}

func Test_getNamespaces_expiredCache(t *testing.T) {

	var namespaces []string
	nsresources := make(map[string][]resource)

	mock_cache := mockNamespaceCache()

	//mock cache for token review to get user data:
	mock_cache.tokenReviews["123456"] = &tokenReviewCache{
		tokenReview: &authv1.TokenReview{
			Status: authv1.TokenReviewStatus{
				User: authv1.UserInfo{
					UID: "unique-user-id",
				},
			},
		},
	}

	//mock cache for cluster-scoped resouces to get all namespaces:
	mock_cache.shared.namespaces = append(namespaces, "some-namespace")
	//mock cache for namespaced-resources:
	nsresources["some-namespace"] = append(nsresources["some-namespace"],
		resource{apigroup: "some-apigroup", kind: "some-kind"})

	mock_cache.users["unique-user-id"] = &userData{
		nsResources:  nsresources,
		nsrUpdatedAt: time.Now().Add(time.Duration(-5) * time.Minute),
	}

	result, err := mock_cache.GetUserData(context.Background(), "123456")

	if len(result.nsResources) == 0 {
		t.Error("Resources not in cache.")
	}
	if err != nil {
		t.Error("Unexpected error while obtaining namespaces.", err)
	}

	// Verify that cache was updated within the last 2 millisecond.
	if mock_cache.users["unique-user-id"].nsrUpdatedAt.After(time.Now().Add(time.Duration(-2) * time.Millisecond)) {
		t.Error("Expected the cache.users.updatedAt to be less than 2 millisecond ago.")
	}

}
