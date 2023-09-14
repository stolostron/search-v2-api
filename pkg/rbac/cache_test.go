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
	res := GetCache()
	assert.Equal(t, res.dbConnInitialized, false)

	res.SetDbConnInitialized(true)

	assert.Equal(t, res.dbConnInitialized, true)
}
