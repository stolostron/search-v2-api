// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sort"
	"strconv"
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
func newMockSearchSchema(t *testing.T) (*SearchSchema, *pgxpoolmock.MockPgxPool) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)

	mockResolver := &SearchSchema{
		pool: mockPool,
	}
	return mockResolver, mockPool
}

func newMockMessage(t *testing.T) (*Message, *pgxpoolmock.MockPgxPool) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)

	mockResolver := &Message{
		pool: mockPool,
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

//Prop will be the property input for searchComplete
func newMockRows(mockDataFile string, input *model.SearchInput, prop string, limit int) *MockRows {
	// Read json file and build mock data
	bytes, _ := ioutil.ReadFile(mockDataFile)
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

	mockData := make([]map[string]interface{}, 0)

	switch prop {
	case "":

		for _, item := range items {
			if !strings.Contains(mockDataFile, "rel") { // load resources file

				if useInputFilterToLoadData(mockDataFile, input, item) {
					uid := item.(map[string]interface{})["uid"]

					mockDatum := map[string]interface{}{
						"uid":      uid,
						"cluster":  strings.Split(uid.(string), "/")[0],
						"data":     item.(map[string]interface{})["properties"],
						"destid":   item.(map[string]interface{})["DestUID"],
						"destkind": item.(map[string]interface{})["DestKind"],
					}

					mockData = append(mockData, mockDatum)
				}

			} else { // load relations file
				mockDatum := map[string]interface{}{
					"level": item.(map[string]interface{})["Level"],
					"uid":   item.(map[string]interface{})["DestUID"],
					"kind":  item.(map[string]interface{})["DestKind"],
				}
				mockData = append(mockData, mockDatum)
			}
		}
	default: // For searchschema and searchComplete
		// For searchComplete
		props := map[string]string{}
		for _, item := range items {
			uid := item.(map[string]interface{})["uid"]
			cluster := strings.Split(uid.(string), "/")[0]
			data := item.(map[string]interface{})["properties"].(map[string]interface{})

			if prop == "cluster" {
				props[cluster] = ""
			} else {
				if _, ok := data[prop]; ok {
					switch v := data[prop].(type) {

					case float64:
						props[strconv.Itoa(int(v))] = ""
					default:
						props[v.(string)] = ""
					}
				}
			}
		}
		mapKeys := []interface{}{}
		for key := range props {
			mapKeys = append(mapKeys, key)
		}

		//if limit is set, sort results and send only the assigned limit
		if limit > 0 && len(mapKeys) >= limit {
			switch mapKeys[0].(type) {
			case string:
				mapKey := make([]string, len(mapKeys))
				for i, v := range mapKeys {
					mapKey[i] = v.(string)
				}
				sort.Strings(mapKey)
				mapKeys = []interface{}{}
				for _, v := range mapKey {
					mapKeys = append(mapKeys, v)
				}
			case int:
				sort.Slice(mapKeys, func(i, j int) bool {
					numA, _ := mapKeys[i].(int)
					numB, _ := mapKeys[j].(int)
					return numA < numB
				})
			}

			mapKeys = mapKeys[:limit]
		}
		for _, key := range mapKeys {
			mockDatum := map[string]interface{}{
				"prop": key,
			}
			mockData = append(mockData, mockDatum)

		}
	}

	return &MockRows{
		mockData:      mockData,
		index:         0,
		columnHeaders: columnHeaders,
	}
}
func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if strings.EqualFold(b, a) {
			return true
		}
	}
	return false
}

// Only load mock data items if the input filters conditions are satisfied
func useInputFilterToLoadData(mockDataFile string, input *model.SearchInput, item interface{}) bool {
	// var destkind string
	var relatedValues []string

	if len(input.RelatedKinds) > 0 {
		relatedValues = pointerToStringArray(input.RelatedKinds)
		data := item.(map[string]interface{})["properties"].(map[string]interface{})
		destkind := data["kind"].(string)
		if stringInSlice(destkind, relatedValues) {
			return true // If the resource kind is not in RelatedKinds, do not load it
		} else {
			return false
		}
	}

	for _, filter := range input.Filters {
		if len(filter.Values) > 0 {
			values := pointerToStringArray(filter.Values) //get the filter values

			opValueMap := getOperatorAndNumDateFilter(values) // get the filter values if property is a number or date
			var op string
			for key, val := range opValueMap {
				op = key
				values = val
			}

			uid := item.(map[string]interface{})["uid"]

			if filter.Property == "cluster" {
				cluster := strings.Split(uid.(string), "/")[0]
				if !stringInSlice(cluster, values) {
					return false // If the filter value is not in resource, do not load it
				}
			} else {
				data := item.(map[string]interface{})["properties"].(map[string]interface{})

				if data[filter.Property] == nil { // if required property is not set, don't load the item
					return false
				}
				var filterValue int
				var err error
				if op != "" {
					filterValue, err = strconv.Atoi(values[0]) // if the property is a number, get the first value
					// It will be a date property if there is error in conversion and operator is ">"
					if err != nil && op != ">" {
						fmt.Println("Error converting value to int", err)
					}
				}

				switch op {
				case "<":
					return int(data[filter.Property].(float64)) < filterValue

				case ">":
					_, ok := data[filter.Property].(float64)
					return ok && int(data[filter.Property].(float64)) > filterValue

				case ">=":
					return int(data[filter.Property].(float64)) >= filterValue
				case "<=":
					return int(data[filter.Property].(float64)) <= filterValue
				case "!", "!=":
					return int(data[filter.Property].(float64)) != filterValue
				case "=":
					return int(data[filter.Property].(float64)) == filterValue
				default:
					// If the filter value is not in resource, do not load it
					return stringInSlice(data[filter.Property].(string), values)
				}
			}
		}
	}
	return true
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
	if len(dest) > 1 { // For search function

		for i := range dest {
			switch v := dest[i].(type) {
			case *int:
				*dest[i].(*int) = int(r.mockData[r.index-1][r.columnHeaders[i]].(float64))
			case *string:
				*dest[i].(*string) = r.mockData[r.index-1][r.columnHeaders[i]].(string)
			case *map[string]interface{}:
				*dest[i].(*map[string]interface{}) = r.mockData[r.index-1][r.columnHeaders[i]].(map[string]interface{})
			case *interface{}:
				dest[i] = r.mockData[r.index-1][r.columnHeaders[i]]
			case nil:
				klog.Info("error type %T", v)
			default:
				klog.Info("unexpected type %T", v)

			}

		}
	} else if len(dest) == 1 { // For searchComplete function and resolveUIDs function
		_, ok := r.mockData[r.index-1]["prop"] //Check if prop is present in mockdata
		if ok {
			*dest[0].(*string) = r.mockData[r.index-1]["prop"].(string)
		} else { //used by resolveUIDs function
			*dest[0].(*string) = r.mockData[r.index-1]["uid"].(string)
		}
	}
	return nil
}

func (r *MockRows) Values() ([]interface{}, error) { return nil, nil }

func (r *MockRows) RawValues() [][]byte { return nil }

func AssertStringArrayEqual(t *testing.T, result, expected []*string, message string) {
	resultSorted := pointerToStringArray(result)
	sort.Strings(resultSorted)
	expectedSorted := pointerToStringArray(expected)
	sort.Strings(expectedSorted)

	for i, exp := range expectedSorted {
		if resultSorted[i] != exp {
			t.Errorf("%s expected [%v] got [%v]", message, expectedSorted, resultSorted)
			return
		}
	}
}
