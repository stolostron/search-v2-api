// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
)

// Mocks the cache object defined in the rbac package.
type MockCache struct {
	disabled map[string]struct{}
	err      error
}

func (mc *MockCache) GetDisabledClusters(ctx context.Context) (*map[string]struct{}, error) {
	return &mc.disabled, mc.err
}
