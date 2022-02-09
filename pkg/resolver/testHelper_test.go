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
		columnHeaders := []string{"uid string", "cluster string", "data *map[string]interface{}"}
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
			uid := item.(map[string]interface{})["uid"]
			mockResources[i] = map[string]interface{}{
				"uid":  uid,
				"data": item.(map[string]interface{})["properties"],
			}
		}
		mockEdgeData := make([]map[string]interface{}, len(edges))
		for i, edge := range edges {
			mockEdgeData[i] = map[string]interface{}{
				"sourceid": edge.(map[string]interface{})["SourceUID"],
				"destid":   edge.(map[string]interface{})["DestUID"],
				"destkind": edge.(map[string]interface{})["DestKind"],
			}
		}
		mockData := append(mockResources, mockEdgeData...)
		columnHeaders := []string{"uid string", "data *map[string]interface{}", "sourceid string", "destid string", "destkind string"}
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
	for _, value := range dest {
		switch v := value.(type) {
		// case int:
		// 	*value.(*int) = r.mockData[r.index-1][v].(int)
		case string:
			*value.(*string) = r.mockData[r.index-1][v].(string)
		case map[string]interface{}:
			*value.(*map[string]interface{}) = r.mockData[r.index-1]["data"].(map[string]interface{})
		}
	}
	return nil
}

func (r *MockRows) Values() ([]interface{}, error) { return nil, nil }

func (r *MockRows) RawValues() [][]byte { return nil }

func mergeMaps(maps ...[]map[string]interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, len(maps))
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}
