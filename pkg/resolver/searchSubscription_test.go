// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"testing"

	"time"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/golang/mock/gomock"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockSearch struct {
	mock.Mock
}

func (m *MockSearch) Search(ctx context.Context, input []*model.SearchInput) ([]*SearchResult, error) {
	args := m.Called(ctx, input)
	return args.Get(0).([]*SearchResult), args.Error(1)
}

func TestSearchSubscription(t *testing.T) {
	config.Cfg.SubscriptionPollTimeout = 1 // Set timeout to 1 minute
	config.Cfg.SubscriptionPollInterval = 2 // Set Poll interval to 2 seconds
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)
	propTypesMock := map[string]string{"kind": "string"}
	ctx := context.WithValue(context.Background(), rbac.ContextAuthTokenKey, "123456")

	val1 := "Pod"
	input := []*model.SearchInput{{Filters: []*model.SearchFilter{{Property: "kind", Values: []*string{&val1}}}}}

	// Set up mock behavior for Search Query
	mockSearch := new(MockSearch)
	mockSearch.On("Search", mock.Anything, input).Return([]*SearchResult{{
		input:     input[0],
		pool:      mockPool,
		uids:      nil,
		userData:  rbac.UserData{CsResources: []rbac.Resource{}},
		propTypes: propTypesMock,
		context:   ctx,
	}}, nil).Once()

	ch, err := SearchSubscription(ctx, input)
	assert.NoError(t, err)

	// Verify the result in channel
	select {
	case result := <-ch:
		assert.Len(t, result, 1)
	case <-time.After(3 * time.Second):
		// Error if no response after exceeding poll time
		t.Fatal("Did not receive result in time")
	}

	// TODO - Test context cancellation
	ctx.Done()
}