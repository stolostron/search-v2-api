// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"sync"
	"testing"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/golang/mock/gomock"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgproto3/v2"
	"github.com/stolostron/search-v2-api/graph/model"
	"k8s.io/klog/v2"
)

func newMockSearchResolver(t *testing.T, input *model.SearchInput, uids []*string) (*SearchResult, *pgxpoolmock.MockPgxPool) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)

	mockResolver := &SearchResult{
		input: input,
		pool:  mockPool,
		uids:  uids,
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
	bytes, _ := ioutil.ReadFile(mockDataFile) //read data into Items struct which is []map[string]interface{}
	var data map[string]interface{}
	if err := json.Unmarshal(bytes, &data); err != nil {
		panic(err)
	}

	columns := data["columns"].([]interface{})
	columnHeaders := make([]string, len(columns))
	for i, col := range columns {
		columnHeaders[i] = col.(string)
	}

	items := data["records"].([]interface{})

	mockData := make([]map[string]interface{}, len(items))

	for i, item := range items {

		mockRow := make(map[string]interface{})

		if item.(map[string]interface{})["properties"] != nil {
			mockRow["data"] = item.(map[string]interface{})["properties"]
		}
		if item.(map[string]interface{})["uid"] != nil {
			uid := item.(map[string]interface{})["uid"]
			mockRow["uid"] = uid
			mockRow["cluster"] = strings.Split(uid.(string), "/")[0]
		}
		if item.(map[string]interface{})["DestUID"] != nil {
			mockRow["destid"] = item.(map[string]interface{})["DestUID"]
		}
		if item.(map[string]interface{})["DestKind"] != nil {
			mockRow["destkind"] = item.(map[string]interface{})["DestKind"]
		}

		mockData[i] = mockRow
	}

	return &MockRows{
		mockData:      mockData,
		index:         -1,
		columnHeaders: columnHeaders,
	}
}

type MockRows struct {
	mockData      []map[string]interface{}
	index         int
	columnHeaders []string
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
	for i := range dest {
		switch v := dest[i].(type) {
		case *int:
			fmt.Printf("Integer value of %+v", v)
			*dest[i].(*int) = r.mockData[r.index][r.columnHeaders[i]].(int)
			klog.Infof("scaned %s", v)
		case *string:
			fmt.Printf("String value is %+v", v)
			*dest[i].(*string) = r.mockData[r.index][r.columnHeaders[i]].(string)
			klog.Infof("scaned %s", v)
		case *map[string]interface{}:
			fmt.Printf("Map value is %+v", v)
			*dest[i].(*map[string]interface{}) = r.mockData[r.index][r.columnHeaders[i]].(map[string]interface{})
			klog.Infof("scaned %s", v)
		case nil:
			fmt.Printf("error type %T", v)
		default:
			fmt.Printf("unexpected type %T", v)

		}

	}
	return nil
}

func (r *MockRows) Values() ([]interface{}, error) { return nil, nil }

func (r *MockRows) RawValues() [][]byte { return nil }
