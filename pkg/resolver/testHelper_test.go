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
	if testType == "non-rel" {

		dataDir := "./mocks/mock.json"
		bytes, _ := ioutil.ReadFile(dataDir)
		var data map[string]interface{}
		if err := json.Unmarshal(bytes, &data); err != nil {
			panic(err)
		}
		items := data["addResources"].([]interface{})
		mockData := make([]map[string]interface{}, len(items))
		for i, item := range items {
			uid := item.(map[string]interface{})["uid"]
			mockData[i] = map[string]interface{}{
				"uid":     uid,
				"cluster": strings.Split(uid.(string), "/")[0],
				"data":    item.(map[string]interface{})["properties"],
			}
		}
		fmt.Println("Mockdata is:", len(mockData))
		fmt.Print("\n")
		// fmt.Println("Mockdata is:", mockData[1])
		// fmt.Print("\n")
		columnHeaders := []string{"uid", "cluster", "data"}
		return &MockRows{
			mockData:      mockData,
			index:         0,
			columnHeaders: columnHeaders,
		}
	} else if testType == "rel" {
		dataDir := "./mocks/mock-rel.json"

		bytes, _ := ioutil.ReadFile(dataDir)
		var data map[string]interface{}
		if err := json.Unmarshal(bytes, &data); err != nil {
			panic(err)
		}
		items := data["addResources"].([]interface{})
		edges := data["addEdges"].([]interface{})

		mockResources := make([]map[string]interface{}, len(items))
		for i, item := range items {
			mockResources[i] = map[string]interface{}{
				// "uid":  uid,
				"data": item.(map[string]interface{})["properties"],
			}
		}
		mockEdgeData := make([]map[string]interface{}, len(edges))
		for i, edge := range edges {
			mockEdgeData[i] = map[string]interface{}{
				// "sourceid": edge.(map[string]interface{})["SourceUID"].(string),
				"destid":   edge.(map[string]interface{})["DestUID"],
				"destkind": edge.(map[string]interface{})["DestKind"],
			}
		}

		mockData := append(mockResources, mockEdgeData...)

		fmt.Println("Mockdata is:", mockData[0])
		fmt.Print("\n")
		fmt.Println("Mockdata is:", mockData[1])
		fmt.Print("\n")
		fmt.Printf("data value in first map in mockData []map[string]interface{} valuetype is %T:", mockData[0]["data"])
		fmt.Print("\n")
		fmt.Printf("destid value in first map in mockData []map[string]interface{} valuetype is %T:", mockData[1]["destid"])
		fmt.Print("\n")
		fmt.Printf("destkind value in first map in mockData []map[string]interface{} valuetype is %T:", mockData[1]["destkind"])
		fmt.Print("\n")
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
	fmt.Println("Something")
	// dest = dest[0]
	for i := range dest {
		if i > len(r.mockData) {
			break
		}
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
			*dest[i].(*map[string]interface{}) = r.mockData[r.index-1][r.columnHeaders[i]].(map[string]interface{})
			klog.Infof("scaned %s", v)
		case nil:
			fmt.Printf("error type %T", v)
		default:
			fmt.Printf("unexpected type %T", v)

		}

	}
	return nil
}

// mockEdgeData[0]["destid"]

// func (r *MockRows) Scan(dest ...interface{}) error {
// 	for _, col := range r.columnHeaders {

// 		fmt.Println("colmns are:", col)

// 		fmt.Printf("The value of dest is:%s", dest[0])
// 		fmt.Printf("The type of dest is: %T", dest)
// 		fmt.Print("\n")
// 	}
// 	for _, de := range dest {
// 		fmt.Println("The value of each object in dest is: ", de)
// 		fmt.Printf("The type of each object in dest is: %T", de)
// 	}

// 	for _, col := range r.columnHeaders {
// 		for _, record := range dest { //for each value in dest which would be for ex: {&data, &destid, &destkind}
// 			klog.Infof(" [===>] Record: %s", record)

// 			if rec, ok := record.(*map[string]interface{}); ok {
// 				fmt.Println("value for rec", record)
// 				for key, val := range *rec {
// 					klog.Infof(" [========>] %s = %s", key, val)
// 					*val.(*map[string]interface{}) = r.mockData[r.index-1]["data"].(map[string]interface{})
// 					klog.Infof("scaned %s", val)

// 				}
// 			} else if rec, ok := record.(*string); ok {
// 				klog.Infof("Rec is %s", *rec)
// 				*record.(*string) = r.mockData[r.index-1][col].(string)
// 				klog.Infof("scaned %s", *rec)

// 			} else if rec, ok := record.(string); ok {
// 				klog.Infof("Rec is %s", rec)
// 				*record.(*string) = r.mockData[r.index-1][col].(string)
// 				klog.Infof("scaned %s", rec)

// 			} else if rec, ok := record.(map[string]interface{}); ok {
// 				for key, val := range rec {
// 					klog.Infof(" [========>] %s = %s", key, val)
// 					*val.(*map[string]interface{}) = r.mockData[r.index-1][col].(map[string]interface{})
// 					klog.Infof("scaned %s", val)
// 				}
// 			} else {
// 				fmt.Printf("record not a map[string]interface{}: %v\n", record)
// 			}
// 		}
// 	}
// 	return nil
// }

// func (r *MockRows) Scan(dest ...*interface{}) error {
// 	for _, mapval := range dest {
// 		for _, val := range *mapval {
// 		for _, col := range r.columnHeaders {
// 			if _, ok := vak.(string); ok {
// 				*vak.(string) = r.mockData[r.index-1][col].(string)
// 			}

// 		}
// 	}
// }
// 	return nil
// }

// 	for _, col := range r.columnHeaders {
// 		// for _, value := range dest {
// 		if _, ok := dest[0].(*int); ok {
// 			fmt.Println("col name:", col)
// 			fmt.Println("Its an int")
// 			*dest[0].(*int) = r.mockData[r.index-1][col].(int)
// 		} else if _, ok := dest[0].(*string); ok {
// 			fmt.Println("col name:", col)
// 			fmt.Println("It's a *string")
// 			*dest[0].(*string) = r.mockData[r.index-1][col].(string)
// 		} else if _, ok := dest[0].(*map[string]interface{}); ok {
// 			fmt.Println("col name:", col)
// 			fmt.Println("It's a *map[string]interface{}")
// 			*dest[0].(*map[string]interface{}) = r.mockData[r.index-1][col].(map[string]interface{})
// 		} else if _, ok := dest[0].(string); ok {
// 			fmt.Println("col name:", col)
// 			fmt.Println("It's a string")
// 			*dest[0].(*string) = r.mockData[r.index-1][col].(string)
// 		} else if _, ok := dest[0].(map[string]interface{}); ok {
// 			fmt.Println("col name:", col)
// 			fmt.Println("It's a map[string]interface{}")
// 			*dest[0].(*map[string]interface{}) = r.mockData[r.index-1][col].(map[string]interface{})
// 		} else if _, ok := dest[0].(error); ok {
// 			fmt.Println("col name:", col)
// 			fmt.Println("It's a nil")
// 			*dest[0].(*error) = r.mockData[r.index-1][col].(error)

// 		}
// 		// }
// 	}
// 	return nil
// }

// func (r *MockRows) Scan(dest ...interface{}) error {
// 	for _, col := range r.columnHeaders {
// 		for _, value := range dest {
// 			fmt.Println("col is\n", col)
// 			fmt.Printf("value is %+v\n", value)

// 		switch v := value.(type) {
// 		case int:
// 			fmt.Println("value of v:", v)
// 			*value.(*int) = r.mockData[r.index-1][col].(int)
// 		case string:
// 			fmt.Println("value of v:", v)
// 			*value.(*string) = r.mockData[r.index-1][col].(string)
// 		case *string:
// 			fmt.Println("value of v:", v)
// 			*value.(*string) = r.mockData[r.index-1][col].(string)
// 		case map[string]interface{}:
// 			fmt.Println("value of v:", v)
// 			*value.(*map[string]interface{}) = r.mockData[r.index-1][col].(map[string]interface{})
// 		case *map[string]interface{}:
// 			fmt.Printf("value is %+v", v)
// 			*value.(*map[string]interface{}) = r.mockData[r.index-1][col].(map[string]interface{})
// 		default:
// 			fmt.Printf("unexpected type %T", v)
// 		}
// 	}
// }

// 		}
// 	}
// 	return nil
// }

func (r *MockRows) Values() ([]interface{}, error) { return nil, nil }

func (r *MockRows) RawValues() [][]byte { return nil }
