// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Cache_GetCache(t *testing.T) {
	res := GetCache()

	assert.Equal(t, res, &cacheInst)
}

func Test_Cache_DBConn(t *testing.T) {
	c := GetCache()
	result := c.GetDbConnInitialized()

	assert.Equal(t, result, false)

	c.setDbConnInitialized(true)

	assert.Equal(t, c.dbConnInitialized, true)
}
