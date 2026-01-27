// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"sync"
	"time"

	clusterviewv1alpha1 "github.com/stolostron/cluster-lifecycle-api/clusterview/v1alpha1"
	authv1 "k8s.io/api/authentication/v1"
	authz "k8s.io/api/authorization/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	authnv1 "k8s.io/client-go/kubernetes/typed/authentication/v1"
	v1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"k8s.io/client-go/rest"
)

// MockAuthzClient mocks the authorization client for testing
type MockAuthzClient struct {
	Permissions map[WatchPermissionKey]*WatchPermissionEntry
}

// RESTClient implements the AuthorizationV1Interface
func (m *MockAuthzClient) RESTClient() rest.Interface {
	return nil
}

// SelfSubjectAccessReviews returns a mock self subject access review interface
func (m *MockAuthzClient) SelfSubjectAccessReviews() v1.SelfSubjectAccessReviewInterface {
	return &mockSelfSubjectAccessReviewInterface{client: m}
}

// SelfSubjectRulesReviews implements the AuthorizationV1Interface
func (m *MockAuthzClient) SelfSubjectRulesReviews() v1.SelfSubjectRulesReviewInterface {
	return nil
}

// SubjectAccessReviews implements the AuthorizationV1Interface
func (m *MockAuthzClient) SubjectAccessReviews() v1.SubjectAccessReviewInterface {
	return nil
}

// LocalSubjectAccessReviews implements the AuthorizationV1Interface
func (m *MockAuthzClient) LocalSubjectAccessReviews(namespace string) v1.LocalSubjectAccessReviewInterface {
	return nil
}

// mockSelfSubjectAccessReviewInterface mocks the SSAR interface
type mockSelfSubjectAccessReviewInterface struct {
	client *MockAuthzClient
}

// Create simulates creating an SSAR
func (m mockSelfSubjectAccessReviewInterface) Create(ctx context.Context, ssar *authz.SelfSubjectAccessReview, opts metav1.CreateOptions) (*authz.SelfSubjectAccessReview, error) {
	key := WatchPermissionKey{
		verb:      ssar.Spec.ResourceAttributes.Verb,
		apigroup:  ssar.Spec.ResourceAttributes.Group,
		kind:      ssar.Spec.ResourceAttributes.Resource,
		namespace: ssar.Spec.ResourceAttributes.Namespace,
	}

	entry, ok := m.client.Permissions[key]
	if !ok {
		ssar.Status.Allowed = false
	} else {
		ssar.Status.Allowed = entry.allowed
	}

	return ssar, nil
}

// MockAuthnClient mocks the authentication client for testing
type MockAuthnClient struct {
	UserInfo authv1.UserInfo
}

func (m *MockAuthnClient) SelfSubjectReviews() authnv1.SelfSubjectReviewInterface {
	return nil
}

// RESTClient implements the AuthenticationV1Interface
func (m *MockAuthnClient) RESTClient() rest.Interface {
	return nil
}

// TokenReviews returns a mock token review interface
func (m *MockAuthnClient) TokenReviews() authnv1.TokenReviewInterface {
	return &mockTokenReviewInterface{userInfo: m.UserInfo}
}

// mockTokenReviewInterface mocks the token review interface
type mockTokenReviewInterface struct {
	userInfo authv1.UserInfo
}

// Create simulates creating a token review
func (m *mockTokenReviewInterface) Create(ctx context.Context, tr *authv1.TokenReview, opts metav1.CreateOptions) (*authv1.TokenReview, error) {
	tr.Status.Authenticated = true
	tr.Status.User = m.userInfo
	return tr, nil
}

// NewMockAuthzClient creates a new mock authorization client
func NewMockAuthzClient() *MockAuthzClient {
	return &MockAuthzClient{
		Permissions: make(map[WatchPermissionKey]*WatchPermissionEntry),
	}
}

// SetupWatchCacheWithUserData sets up the watch cache with user data for testing
func SetupWatchCacheWithUserData(ctx context.Context, userData *UserWatchData) {
	watchCache := GetWatchCache()
	cache := GetCache()
	uid, _ := cache.GetUserUID(ctx)

	watchCache.watchUserDataLock.Lock()
	defer watchCache.watchUserDataLock.Unlock()

	if watchCache.watchUserData == nil {
		watchCache.watchUserData = make(map[string]*UserWatchData)
	}

	watchCache.watchUserData[uid] = userData
}

// CleanupWatchCache cleans up the watch cache for testing
func CleanupWatchCache(ctx context.Context) {
	watchCache := GetWatchCache()
	cache := GetCache()
	uid, _ := cache.GetUserUID(ctx)

	watchCache.watchUserDataLock.Lock()
	defer watchCache.watchUserDataLock.Unlock()

	if watchCache.watchUserData != nil {
		delete(watchCache.watchUserData, uid)
	}
}

// CreateTestUserWatchData creates test user watch data with the given permissions
func CreateTestUserWatchData(verb, apigroup, kind, namespace string, allowed bool, updatedAt time.Time, ttl time.Duration) *UserWatchData {
	mockClient := NewMockAuthzClient()
	permissions := map[WatchPermissionKey]*WatchPermissionEntry{
		WatchPermissionKey{
			verb:      verb,
			apigroup:  apigroup,
			kind:      kind,
			namespace: namespace,
		}: &WatchPermissionEntry{
			allowed:   allowed,
			updatedAt: updatedAt,
		},
	}
	mockClient.Permissions = permissions

	return &UserWatchData{
		authzClient:     mockClient,
		permissions:     make(map[WatchPermissionKey]*WatchPermissionEntry),
		permissionsLock: sync.RWMutex{},
		ttl:             ttl,
	}
}

// CreateTestContext creates a test context with the given user info
func CreateTestContext(userUID, username string) context.Context {
	ctx := context.Background()

	// set up the user info in the regular RBAC cache so getUserUidFromContext works
	cache := GetCache()
	userInfo := authv1.UserInfo{
		UID:      userUID,
		Username: username,
	}

	// Set up mock authentication client
	cache.authnClient = &MockAuthnClient{UserInfo: userInfo}

	token := "test-token-" + userUID
	ctx = context.WithValue(ctx, ContextAuthTokenKey, token)

	return ctx
}

// SetupCacheWithUserData sets up the regular cache with UserDataCache for testing
func SetupCacheWithUserData(ctx context.Context, userData *UserDataCache) {
	cache := GetCache()
	uid, _ := cache.GetUserUID(ctx)

	cache.usersLock.Lock()
	defer cache.usersLock.Unlock()

	if cache.users == nil {
		cache.users = make(map[string]*UserDataCache)
	}

	cache.users[uid] = userData
}

// CleanupCache cleans up the regular cache for testing
func CleanupCache(ctx context.Context) {
	cache := GetCache()
	uid, _ := cache.GetUserUID(ctx)

	cache.usersLock.Lock()
	defer cache.usersLock.Unlock()

	if cache.users != nil {
		delete(cache.users, uid)
	}
}

// CreateTestUserDataCache creates test UserDataCache with the given permissions for fine-grained RBAC
func CreateTestUserDataCache(verb, apigroup, kind, cluster, namespace string) *UserDataCache {
	permissions := clusterviewv1alpha1.UserPermissionList{
		Items: []clusterviewv1alpha1.UserPermission{
			{
				Status: clusterviewv1alpha1.UserPermissionStatus{
					Bindings: []clusterviewv1alpha1.ClusterBinding{
						{Cluster: cluster, Namespaces: []string{namespace}},
					},
					ClusterRoleDefinition: clusterviewv1alpha1.ClusterRoleDefinition{
						Rules: []rbacv1.PolicyRule{
							{Verbs: []string{verb}, APIGroups: []string{apigroup}, Resources: []string{kind}},
						},
					},
				},
			},
		},
	}

	return &UserDataCache{
		UserData: UserData{
			IsClusterAdmin:  false,
			UserPermissions: permissions,
		},
		// Make cache valid to skip API call
		clustersCache:       cacheMetadata{updatedAt: time.Now()},
		csrCache:            cacheMetadata{updatedAt: time.Now()},
		nsrCache:            cacheMetadata{updatedAt: time.Now()},
		userPermissionCache: cacheMetadata{updatedAt: time.Now()},
	}
}
