// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	// "reflect"
	"sync"
	"testing"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/golang/mock/gomock"
	"github.com/stolostron/search-v2-api/graph/model"
	// "k8s.io/klog/v2"
)

func Test_SearchResolver_Count(t *testing.T) {
	// Mock the database connection
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)

	mockRow := &Row{MockValue: 10}
	mockPool.EXPECT().QueryRow(gomock.Any(),
		gomock.Eq("SELECT count(uid) FROM search.resources  WHERE lower(data->> 'kind')=$1"),
		gomock.Eq("pod")).Return(mockRow)

	// Build search resolver
	val1 := "pod"
	resolver := &SearchResult{
		pool: mockPool,
		// Filter 'kind:pod'
		input: &model.SearchInput{
			Filters: []*model.SearchFilter{
				&model.SearchFilter{Property: "kind", Values: []*string{&val1}},
			},
		},
	}

	// Execute function
	r := resolver.Count()

	// Verify response
	if r != mockRow.MockValue {
		t.Errorf("Incorrect Count() expected [%d] got [%d]", mockRow.MockValue, r)
	}
}

func Test_SearchResolver_Items(t *testing.T) {
	// Mock the database connection
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)

	// get mock data for items
	mockRows := BuildMockRows("./mocks/mock.json")

	mockPool.EXPECT().Query(gomock.Any(),
		gomock.Eq("SELECT uid, cluster, data FROM search.resources  WHERE lower(data->> 'kind')=$1"),
		gomock.Eq("template"),
	).Return(mockRows, nil) //return above query with kind=template should return mock data

	// Build search resolver
	val1 := "Template"
	resolver := &SearchResult{
		input: &model.SearchInput{Filters: []*model.SearchFilter{&model.SearchFilter{Property: "kind", Values: []*string{&val1}}}},
		pool:  mockPool,
		uids:  []string{},
		wg:    sync.WaitGroup{},
	}

	// Execute function
	r := resolver.Items()

	// FIXME: Verify response
	// eq := reflect.DeepEqual(r, mockRows.mockData)
	// if eq {
	// 	klog.Info("correct items")
	// } else {
	// 	t.Errorf("Incorrect Items() expected [%+v] got [%+v]", mockRows.mockData, r)
	// }

	// Simple verification
	if len(r) != len(mockRows.mockData) {
		t.Errorf("Items() received incorrect number of items. Got [%d] Expected [%d]", len(r), len(mockRows.mockData))
	}
}
