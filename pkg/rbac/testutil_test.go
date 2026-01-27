package rbac

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	authz "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCreateTestContext(t *testing.T) {
	// Given: a username and userUID
	userUID := "john-123"
	username := "john"

	// When: we pass them to our function to build a context with those
	ctx := CreateTestContext(userUID, username)

	// Then: it's setup
	assert.Equal(t, ctx.Value(ContextAuthTokenKey), "test-token-"+userUID)
}

func TestCreateTestUserWatchDataTest(t *testing.T) {
	// Given: some permission fields
	verb := "watch"
	apigroup := "kubevirt.io"
	kind := "virtualmachines"
	namespace := "foo"
	allowed := true
	updatedAt := time.Now()
	ttl := 1 * time.Second

	// When: we pass them to our function to build the UserWatchData
	userWatchData := CreateTestUserWatchData(verb, apigroup, kind, namespace, allowed, updatedAt, ttl)

	// Then: it's made
	assert.NotNil(t, userWatchData)
	assert.Equal(t, userWatchData.ttl, ttl)
}

func TestCreateTestUserDataCache(t *testing.T) {
	// Given: some permission fields
	verb := "watch"
	apigroup := "kubevirt.io"
	kind := "virtualmachines"
	namespace := "foo"
	cluster := "managed-cluster"

	// When: we pass them to our function to build the UserDataCache
	userDataCache := CreateTestUserDataCache(verb, apigroup, kind, cluster, namespace)

	// Then: it's made
	assert.NotNil(t, userDataCache)
	assert.Equal(t, len(userDataCache.UserPermissions.Items), 1)
}

func TestMockSelfSubjectAccessReviewInterface_Create_Allowed(t *testing.T) {
	// Given: a mock client with a permission that allows watch on pods in namespace foo
	mockClient := NewMockAuthzClient()
	mockClient.Permissions[WatchPermissionKey{
		verb:      "watch",
		apigroup:  "v1",
		kind:      "pods",
		namespace: "foo",
	}] = &WatchPermissionEntry{
		allowed:   true,
		updatedAt: time.Now(),
	}

	mockInterface := mockSelfSubjectAccessReviewInterface{client: mockClient}

	// When: we create an SSAR for a matching permission
	ssar := &authz.SelfSubjectAccessReview{
		Spec: authz.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authz.ResourceAttributes{
				Verb:      "watch",
				Group:     "v1",
				Resource:  "pods",
				Namespace: "foo",
			},
		},
	}

	result, err := mockInterface.Create(context.Background(), ssar, metav1.CreateOptions{})

	// Then: permission should be granted
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, result.Status.Allowed, true, "Expected permission to be allowed")
}

func TestMockSelfSubjectAccessReviewInterface_Create_Denied(t *testing.T) {
	// Given: a mock client with a permission that denies delete on pods in namespace foo
	mockClient := NewMockAuthzClient()
	mockClient.Permissions[WatchPermissionKey{
		verb:      "delete",
		apigroup:  "v1",
		kind:      "pods",
		namespace: "foo",
	}] = &WatchPermissionEntry{
		allowed:   false,
		updatedAt: time.Now(),
	}

	mockInterface := mockSelfSubjectAccessReviewInterface{client: mockClient}

	// When: we create an SSAR for the denied permission
	ssar := &authz.SelfSubjectAccessReview{
		Spec: authz.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authz.ResourceAttributes{
				Verb:      "delete",
				Group:     "v1",
				Resource:  "pods",
				Namespace: "foo",
			},
		},
	}

	result, err := mockInterface.Create(context.Background(), ssar, metav1.CreateOptions{})

	// Then: permission should be denied
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, result.Status.Allowed, false, "Expected permission to be denied")
}

func TestMockSelfSubjectAccessReviewInterface_Create_NotFound(t *testing.T) {
	// Given: a mock client with no permissions set
	mockClient := NewMockAuthzClient()
	mockInterface := mockSelfSubjectAccessReviewInterface{client: mockClient}

	// When: we create an SSAR for a permission that doesn't exist
	ssar := &authz.SelfSubjectAccessReview{
		Spec: authz.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authz.ResourceAttributes{
				Verb:      "watch",
				Group:     "v1",
				Resource:  "pods",
				Namespace: "foo",
			},
		},
	}

	result, err := mockInterface.Create(context.Background(), ssar, metav1.CreateOptions{})

	// Then: permission should be denied (default behavior when permission not found)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, result.Status.Allowed, false, "Expected permission to be denied when not found")
}
