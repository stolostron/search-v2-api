package resolver

import (
	"testing"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/golang/mock/gomock"
	"github.com/stolostron/search-v2-api/graph/model"
)

type Row struct {
	MockValue int
}

func (r *Row) Scan(dest ...interface{}) error {
	*dest[0].(*int) = r.MockValue
	return nil
}

func Test_SearchResolver_Count(t *testing.T) {
	// Mock the database connection
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)

	mockRow := &Row{MockValue: 10}
	mockPool.EXPECT().QueryRow(gomock.Any(),
		gomock.Eq("SELECT count(uid) FROM resources  WHERE lower(data->> 'kind')=$1"),
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
