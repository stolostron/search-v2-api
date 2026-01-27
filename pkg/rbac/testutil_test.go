package rbac

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
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
