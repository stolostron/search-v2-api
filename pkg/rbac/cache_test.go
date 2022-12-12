// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_Cache_GetCache(t *testing.T) {
	res := GetCache()

	assert.Equal(t, res, &cacheInst)
}
