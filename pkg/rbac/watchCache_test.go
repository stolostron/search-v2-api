// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stretchr/testify/assert"
	authv1 "k8s.io/api/authentication/v1"
	authz "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fake "k8s.io/client-go/kubernetes/fake"
	testingk8s "k8s.io/client-go/testing"
)

// [AI] Helper function to initialize watch cache for tests
func mockWatchCache() *WatchCache {
	return &WatchCache{
		watchUserData:       map[string]*UserWatchData{},
		watchUserDataLock:   sync.Mutex{},
		watchCacheUpdatedAt: time.Now(),
	}
}

// [AI] Helper function to setup token for watch cache tests
func setupWatchToken(cache *Cache) *Cache {
	if cache.tokenReviews == nil {
		cache.tokenReviews = map[string]*tokenReviewCache{}
	}
	cache.tokenReviews["watch-token-123"] = &tokenReviewCache{
		meta:       cacheMetadata{updatedAt: time.Now()},
		authClient: fake.NewSimpleClientset().AuthenticationV1(),
		tokenReview: &authv1.TokenReview{
			Status: authv1.TokenReviewStatus{
				User: authv1.UserInfo{
					UID:      "watch-user-id",
					Username: "watch-test-user",
					Groups:   []string{"system:authenticated"},
				},
			},
		},
	}
	return cache
}

func TestWatchCacheGetWatchCache(t *testing.T) {
	res := GetWatchCache()
	assert.Equal(t, res, &watchCacheInst)
}

// [AI]
func Test_CheckPermissionAndCache_CacheHit(t *testing.T) {
	userData := &UserWatchData{
		permissions:     make(map[WatchPermissionKey]*WatchPermissionEntry),
		permissionsLock: sync.RWMutex{},
		ttl:             5 * time.Minute,
	}

	// Pre-populate cache with a valid, non-expired entry
	key := WatchPermissionKey{
		verb:      "watch",
		apigroup:  "apps",
		kind:      "deployments",
		namespace: "default",
	}
	userData.permissions[key] = &WatchPermissionEntry{
		allowed:   true,
		updatedAt: time.Now(),
	}

	ctx := context.Background()
	result := userData.CheckPermissionAndCache(ctx, "watch", "apps", "deployments", "default")

	assert.True(t, result, "Expected cached permission to be true")
	assert.Equal(t, 1, len(userData.permissions), "Should still have only 1 cached entry")
}

// [AI]
func Test_CheckPermissionAndCache_CacheMissExpired(t *testing.T) {
	// Setup fake clientset with SSAR reactor
	fs := fake.NewSimpleClientset()

	fs.PrependReactor("create", "selfsubjectaccessreviews", func(action testingk8s.Action) (handled bool, ret runtime.Object, err error) {
		ret = action.(testingk8s.CreateAction).GetObject()
		ssar := ret.(*authz.SelfSubjectAccessReview)

		// Create response based on the resource being checked
		response := &authz.SelfSubjectAccessReview{
			Spec: ssar.Spec,
			Status: authz.SubjectAccessReviewStatus{
				Allowed: ssar.Spec.ResourceAttributes.Resource == "deployments",
			},
		}
		return true, response, nil
	})

	userData := &UserWatchData{
		authzClient:     fs.AuthorizationV1(),
		permissions:     make(map[WatchPermissionKey]*WatchPermissionEntry),
		permissionsLock: sync.RWMutex{},
		ttl:             100 * time.Millisecond,
	}

	// Test cache miss - permission allowed
	ctx := context.Background()
	result := userData.CheckPermissionAndCache(ctx, "watch", "apps", "deployments", "default")

	assert.True(t, result, "Expected permission to be allowed for deployments")
	assert.Equal(t, 1, len(userData.permissions), "Should have 1 cached entry after first call")

	// Test cache miss - permission denied
	result2 := userData.CheckPermissionAndCache(ctx, "watch", "", "pods", "default")
	assert.False(t, result2, "Expected permission to be denied for pods")
	assert.Equal(t, 2, len(userData.permissions), "Should have 2 cached entries")

	// Test expired cache scenario
	time.Sleep(150 * time.Millisecond) // Wait for TTL to expire
	result3 := userData.CheckPermissionAndCache(ctx, "watch", "apps", "deployments", "default")
	assert.True(t, result3, "Expected permission to still be allowed after TTL expiration")
	assert.Equal(t, 2, len(userData.permissions), "Should still have 2 entries (updated existing)")
}

// [AI]
func Test_GetUserWatchDataCache_NewUser(t *testing.T) {
	// Setup the regular cache for token review
	regularCache := mockNamespaceCache()
	regularCache = setupWatchToken(regularCache)
	// copy just tokenReviews to cache so userUID and userInfo can be got from cache's tokenReview to build authzClient
	cacheInst.tokenReviews = regularCache.tokenReviews

	watchCache := mockWatchCache()

	// Create mock authz client
	fs := fake.NewSimpleClientset()
	authzClient := fs.AuthorizationV1()

	ctx := context.WithValue(context.Background(), ContextAuthTokenKey, "watch-token-123")

	// Get user watch data for a new user
	result, err := watchCache.GetUserWatchDataCache(ctx, authzClient)

	assert.Nil(t, err, "Should not return error for new user")
	assert.NotNil(t, result, "Should return UserWatchData")
	assert.NotNil(t, result.authzClient, "AuthzClient should be set")
	assert.NotNil(t, result.permissions, "Permissions map should be initialized")
	assert.Equal(t, time.Duration(config.Cfg.UserCacheTTL)*time.Millisecond, result.ttl, "TTL should be set from config")
	assert.Equal(t, 1, len(watchCache.watchUserData), "Should have 1 user in cache")
}

// [AI]
func Test_GetUserWatchDataCache_ExistingUser(t *testing.T) {
	// Setup the regular cache for token review
	regularCache := mockNamespaceCache()
	regularCache = setupWatchToken(regularCache)
	// copy just tokenReviews to cache so userUID and userInfo can be got from cache's tokenReview to build authzClient
	cacheInst.tokenReviews = regularCache.tokenReviews

	watchCache := mockWatchCache()

	// Pre-populate with existing user data
	existingUserData := &UserWatchData{
		authzClient:     fake.NewSimpleClientset().AuthorizationV1(),
		permissions:     make(map[WatchPermissionKey]*WatchPermissionEntry),
		permissionsLock: sync.RWMutex{},
		ttl:             10 * time.Minute,
	}
	watchCache.watchUserData["watch-user-id"] = existingUserData

	ctx := context.WithValue(context.Background(), ContextAuthTokenKey, "watch-token-123")

	// Get user watch data for existing user
	result, err := watchCache.GetUserWatchDataCache(ctx, nil)

	assert.Nil(t, err, "Should not return error for existing user")
	assert.Equal(t, existingUserData, result, "Should return the same UserWatchData instance")
	assert.Equal(t, 1, len(watchCache.watchUserData), "Should still have only 1 user in cache")
}

// [AI]
func Test_GetUserWatchData_WithImpersonation(t *testing.T) {
	// Setup the regular cache for token review
	regularCache := mockNamespaceCache()
	regularCache = setupWatchToken(regularCache)
	// copy just tokenReviews to cache so userUID and userInfo can be got from cache's tokenReview to build authzClient
	cacheInst.tokenReviews = regularCache.tokenReviews

	watchCache := mockWatchCache()

	ctx := context.WithValue(context.Background(), ContextAuthTokenKey, "watch-token-123")

	// Call GetUserWatchData (not GetUserWatchDataCache) which passes nil for authzClient
	// This will trigger createImpersonationClient path
	result, err := watchCache.GetUserWatchData(ctx)

	assert.Nil(t, err, "Should not return error")
	assert.NotNil(t, result, "Should return UserWatchData")
	assert.NotNil(t, result.authzClient, "AuthzClient should be created via impersonation")
	assert.NotNil(t, result.permissions, "Permissions map should be initialized")
	assert.Equal(t, time.Duration(config.Cfg.UserCacheTTL)*time.Millisecond, result.ttl, "TTL should be set from config")
	assert.Equal(t, 1, len(watchCache.watchUserData), "Should have 1 user in cache")
}
