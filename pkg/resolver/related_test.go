package resolver

// import (
// 	"testing"

// 	"github.com/driftprogramming/pgxpoolmock"
// 	"github.com/golang/mock/gomock"
// 	"github.com/stolostron/search-v2-api/graph/model"
// )

// type Row struct {
// 	MockValue int
// }

// //making a custom scan function to
// func (r *Row) Scan(dest ...interface{}) error {
// 	*dest[0].(*int) = r.MockValue
// 	return nil
// }

// func Test_SearchResolver_Relations(t *testing.T) {
// 	// Mock the database connection
// 	ctrl := gomock.NewController(t)
// 	defer ctrl.Finish()
// 	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)

// 	// does 10 represent the index of data at hand?
// 	mockRow := &Row{MockValue: 10}
// 	mockPool.EXPECT().QueryRow(gomock.Any(),
// 		gomock.Eq(
// 			"with recursive
// 		search_graph(uid, data, sourcekind, destkind, sourceid, destid, path, level)
// 		as (
// 		SELECT r.uid, r.data, e.sourcekind, e.destkind, e.sourceid, e.destid, ARRAY[r.uid] as path, 1 as level
// 			from search.resources r
// 			INNER JOIN
// 				search.edges e ON (r.uid = e.sourceid) or (r.uid = e.destid)
// 			 where r.uid = ANY($1)
// 		union
// 		select r.uid, r.data, e.sourcekind, e.destkind, e.sourceid, e.destid, path||r.uid, level+1 as level
// 			from search.resources r
// 			INNER JOIN
// 				search.edges e ON (r.uid = e.sourceid)
// 			, search_graph sg
// 			where (e.sourceid = sg.destid or e.destid = sg.sourceid)
// 			and r.uid <> all(sg.path)
// 			and level = 1
// 			)
// 		select distinct on (destid) data, destid, destkind from search_graph where level=1 or destid = ANY($2)"),
// 		gomock.Eq("pod")).Return(mockRow)

// 	// Build search resolver
// 	val1 := "pod"
// 	resolver := &SearchResult{
// 		pool: mockPool,
// 		// Filter 'kind:pod'
// 		input: &model.SearchInput{
// 			Filters: []*model.SearchFilter{
// 				&model.SearchFilter{Property: "kind", Values: []*string{&val1}},
// 			},
// 		},
// 	}

// 	// Execute function
// 	r := resolver.Kind()

// 	// Verify response
// 	if r != mockRow.MockValue {
// 		t.Errorf("Incorrect Related expected [%d] got [%d]", mockRow.MockValue, r)
// 	}
// }
