// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"encoding/json"
	"io/ioutil"
	"strings"
	"sync"
	"testing"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/golang/mock/gomock"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgproto3/v2"
	"github.com/stolostron/search-v2-api/graph/model"
)

func newMockSearchResolver(t *testing.T, input *model.SearchInput) (*SearchResult, *pgxpoolmock.MockPgxPool) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)

	mockResolver := &SearchResult{
		input: input,
		pool:  mockPool,
		uids:  []string{},
		wg:    sync.WaitGroup{},
	}

	return mockResolver, mockPool
}

// ====================================================
// Mock the Row interface defined in the pgx library.
// https://github.com/jackc/pgx/blob/master/rows.go#L24
// ====================================================
type Row struct {
	MockValue int
}

func (r *Row) Scan(dest ...interface{}) error {
	*dest[0].(*int) = r.MockValue
	return nil
}

// ====================================================
// Mock the Rows interface defined in the pgx library.
// https://github.com/jackc/pgx/blob/master/rows.go#L24
// ====================================================

func newMockRows(mockDataFile string) *MockRows {
	// Read json file and build mock data
	bytes, _ := ioutil.ReadFile("./mocks/mock.json") //read data into Items struct which is []map[string]interface{}
	var resources map[string]interface{}
	if err := json.Unmarshal(bytes, &resources); err != nil {
		panic(err)
	}

	items := resources["addResources"].([]interface{})

	mockData := make([]map[string]interface{}, len(items))
	for i, item := range items {
		uid := item.(map[string]interface{})["uid"]
		mockData[i] = map[string]interface{}{
			"uid":     uid,
			"cluster": strings.Split(uid.(string), "/")[0],
			"data":    item.(map[string]interface{})["properties"],
		}
	}

	return &MockRows{
		index:    0,
		mockData: mockData,
	}
}

type MockRows struct {
	mockData []map[string]interface{}
	index    int
}

func (r *MockRows) Close() {}

func (r *MockRows) Err() error { return nil }

func (r *MockRows) CommandTag() pgconn.CommandTag { return nil }

func (r *MockRows) FieldDescriptions() []pgproto3.FieldDescription { return nil }

func (r *MockRows) Next() bool {
	r.index = r.index + 1
	return r.index <= len(r.mockData)
}

func (r *MockRows) Scan(dest ...interface{}) error {
	*dest[0].(*string) = r.mockData[r.index-1]["uid"].(string)
	*dest[1].(*string) = r.mockData[r.index-1]["cluster"].(string)
	*dest[2].(*map[string]interface{}) = r.mockData[r.index-1]["data"].(map[string]interface{})
	return nil
}

func (r *MockRows) Values() ([]interface{}, error) { return nil, nil }

func (r *MockRows) RawValues() [][]byte { return nil }
