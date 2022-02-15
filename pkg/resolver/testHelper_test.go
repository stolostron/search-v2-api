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

func newMockRows(testType string) *MockRows {
	bytes, _ := ioutil.ReadFile("./mocks/mock-rel-1.json")
	var data map[string]interface{}
	if err := json.Unmarshal(bytes, &data); err != nil {
		panic(err)
	}
	items := data["addRecords"].([]interface{})

	mockData := make([]map[string]interface{}, len(items))

	if testType == "non-rel" {

		for i, item := range items {
			uid := item.(map[string]interface{})["uid"]
			mockData[i] = map[string]interface{}{
				"uid":     uid,
				"cluster": strings.Split(uid.(string), "/")[0],
				"data":    item.(map[string]interface{})["properties"],
			}
		}
		fmt.Println("MockData[0]:", mockData[0]["uid"], mockData[0]["cluster"], mockData[0]["data"])
		fmt.Println("MockData[1]:", mockData[1]["uid"], mockData[1]["cluster"], mockData[1]["data"])
		columnHeaders := []string{"uid", "cluster", "data"}
		return &MockRows{
			mockData:      mockData,
			index:         -1,
			columnHeaders: columnHeaders,
		}

	} else if testType == "rel" {
		for i, item := range items {
			destid := item.(map[string]interface{})["DestUID"]
			destkind := item.(map[string]interface{})["DestKind"]
			mockData[i] = map[string]interface{}{
				"data":     item.(map[string]interface{})["properties"],
				"destid":   destid,
				"destkind": destkind,
			}
		}
		columnHeaders := []string{"data", "destid", "destkind"}
		return &MockRows{
			mockData:      mockData,
			index:         0,
			columnHeaders: columnHeaders,
		}

	} else {
		return nil
	}

}

// 	if testType == "non-rel" {

// 		dataDir := "./mocks/mock.json"
// 		bytes, _ := ioutil.ReadFile(dataDir)
// 		var data map[string]interface{}
// 		if err := json.Unmarshal(bytes, &data); err != nil {
// 			panic(err)
// 		}
// 		items := data["addResources"].([]interface{})
// 		mockData := make([]map[string]interface{}, len(items))
// 		for i, item := range items {
// 			uid := item.(map[string]interface{})["uid"]
// 			mockData[i] = map[string]interface{}{
// 				"uid":     uid,
// 				"cluster": strings.Split(uid.(string), "/")[0],
// 				"data":    item.(map[string]interface{})["properties"],
// 			}
// 		}
// 		columnHeaders := []string{"uid", "cluster", "data"}
// 		return &MockRows{
// 			mockData:      mockData,
// 			index:         0,
// 			columnHeaders: columnHeaders,
// 		}
// 	} else if testType == "rel" {
// 		dataDir := "./mocks/mock-rel.json"

// 		bytes, _ := ioutil.ReadFile(dataDir)
// 		var data map[string]interface{}
// 		if err := json.Unmarshal(bytes, &data); err != nil {
// 			panic(err)
// 		}
// 		items := data["addResources"].([]interface{})
// 		edges := data["addEdges"].([]interface{})

// 		mockResources := make([]map[string]interface{}, len(items))
// 		for i, item := range items {
// 			mockResources[i] = map[string]interface{}{
// 				"data": item.(map[string]interface{})["properties"],
// 			}
// 		}
// 		mockEdgeData := make([]map[string]interface{}, len(edges))
// 		for i, edge := range edges {
// 			mockEdgeData[i] = map[string]interface{}{
// 				"destid":   edge.(map[string]interface{})["DestUID"],
// 				"destkind": edge.(map[string]interface{})["DestKind"],
// 			}
// 		}

// 		mockData := append(mockResources, mockEdgeData...)
// 		columnHeaders := []string{"data", "destid", "destkind"}
// 		return &MockRows{
// 			mockData:      mockData,
// 			index:         0,
// 			columnHeaders: columnHeaders,
// 		}

// 	} else {
// 		return nil
// 	}
// }

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
		// if i > len(r.mockData) {
		// 	break
		// }
		switch v := dest[i].(type) {
		case *int:
			fmt.Printf("Integer value of %+v", v)
			*dest[i].(*int) = r.mockData[r.index][r.columnHeaders[i]].(int)
			klog.Infof("scaned %s", v)
		case *string:
			fmt.Printf("String value of %+v", v)
			fmt.Println(" col is", r.columnHeaders[i], i)
			*dest[i].(*string) = r.mockData[r.index][r.columnHeaders[i]].(string)
			klog.Infof("scaned %s", v)
		case *map[string]interface{}:
			fmt.Printf("map value is %+v", v)
			fmt.Println(" col is", r.columnHeaders[i], i)
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
