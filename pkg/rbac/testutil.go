// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"sync"
	"time"

	authv1 "k8s.io/api/authentication/v1"
	authz "k8s.io/api/authorization/v1"
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
