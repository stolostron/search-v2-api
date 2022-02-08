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
		uids:  []*string{},
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
		return &MockRows{
			mockData: mockData,
			index:    0,
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

		findEdges := func(sourceUID string) string {
			result := make(map[string][]string)
			for _, edge := range edges {
				edgeMap := edge.(map[string]interface{})
				if edgeMap["SourceUID"] == sourceUID {
					edgeType := edgeMap["EdgeType"].(string)
					destUIDs, exist := result[edgeType]
					if exist {
						result[edgeType] = append(destUIDs, edgeMap["DestUID"].(string))
					} else {
						result[edgeType] = []string{edgeMap["DestUID"].(string)}
					}
				}
			}
			edgeJSON, _ := json.Marshal(result)
			return string(edgeJSON)
		}

		mockData := make([]map[string]interface{}, len(items))
		for i, item := range items {
			uid := item.(map[string]interface{})["uid"]
			e := findEdges(uid.(string))
			mockData[i] = map[string]interface{}{
				"uid":     uid,
				"cluster": strings.Split(uid.(string), "/")[0],
				"data":    item.(map[string]interface{})["properties"],
				"edges":   e,
			}
		}
		mockEdgeData := make([]map[string]interface{}, len(edges))
		for i, edge := range edges {
			mockEdgeData[i] = map[string]interface{}{
				"edgeType":  edge.(map[string]interface{})["EdgeType"],
				"sourceuid": edge.(map[string]interface{})["SourceUID"],
				"destuid":   edge.(map[string]interface{})["DestUID"],
			}
		}

		allMockData := make([]map[string]interface{}, len(items)+len(edges))
		allMockData = append(allMockData, mockData...)
		allMockData = append(allMockData, mockEdgeData...)

		return &MockRows{
			mockData: allMockData,
			index:    0,
		}

	} else {
		return nil
	}
}

type MockRowsEdges struct {
	mockData []map[string]interface{}
	index    int
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
	*dest[3].(*string) = r.mockData[r.index-1]["destid"].(string)
	*dest[4].(*string) = r.mockData[r.index-1]["destkind"].(string)

	return nil
}

func (r *MockRows) Values() ([]interface{}, error) { return nil, nil }

func (r *MockRows) RawValues() [][]byte { return nil }
