// Copyright Contributors to the Open Cluster Management project
package schema

import (
	"encoding/json"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/golang/mock/gomock"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgproto3/v2"
	"github.com/stolostron/search-v2-api/graph/model"
)

func newMockSearchComplete(t *testing.T, input *model.SearchInput, property string) (*SearchCompleteResult, *pgxpoolmock.MockPgxPool) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)

	mockResolver := &SearchCompleteResult{
		input:    input,
		pool:     mockPool,
		property: property,
	}
	return mockResolver, mockPool
}

// ====================================================
// Mock the Row interface defined in the pgx library.
// https://github.com/jackc/pgx/blob/master/rows.go#L24
// ====================================================
type Row struct {
	MockValue []*string
}

func (r *Row) Scan(dest ...interface{}) error {
	dest[0] = r.MockValue //Sherin: change
	return nil
}

// ====================================================
// Mock the Rows interface defined in the pgx library.
// https://github.com/jackc/pgx/blob/master/rows.go#L24
// ====================================================

func newMockRows(mockDataFile string) *MockRows {
	// Read json file and build mock data
	bytes, _ := ioutil.ReadFile("../resolver/mocks/mock.json") //read data into Items struct which is []map[string]interface{}
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
	dataMap := r.mockData[r.index-1]["data"].(map[string]interface{})
	*dest[0].(*string) = dataMap["kind"].(string)
	return nil
}

func (r *MockRows) Values() ([]interface{}, error) { return nil, nil }

func (r *MockRows) RawValues() [][]byte { return nil }
