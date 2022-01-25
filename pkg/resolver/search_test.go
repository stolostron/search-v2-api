package resolver

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"reflect"
	"sync"
	"testing"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/golang/mock/gomock"
	"github.com/stolostron/search-v2-api/graph/model"
	"k8s.io/klog/v2"
)

type Row struct {
	MockValue int
}

type Items struct {
	MockValue []map[string]interface{}
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
		gomock.Eq("SELECT count(uid) FROM search.resources  WHERE lower(data->> 'kind')=$1"),
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

func Test_SearchResolver_Items(t *testing.T) {
	// Mock the database connection
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)

	// get mock data for items
	bytes, _ := ioutil.ReadFile("./mocks/mock.json") //read data into Items struct which is []map[string]interface{}
	var resources map[string]interface{}
	if err := json.Unmarshal(bytes, &resources); err != nil {
		panic(err)
	}

	items := resources["addResources"].([]interface{})

	addResources := make([]map[string]interface{}, len(items))
	for i, item := range items {
		uid := item.(map[string]interface{})["uid"]
		properties := item.(map[string]interface{})["properties"]
		data, _ := json.Marshal(properties)
		addResources[i] = map[string]interface{}{"uid": uid, "data": string(data)}
	}

	mockRow := &Items{MockValue: addResources}

	mockPool.EXPECT().QueryRow(gomock.Any(),
		gomock.Eq("SELECT data FROM search.resources WHERE lower(data->> 'kind')=$1"),

		gomock.Eq("Template")).Return(mockRow) //return above query with kind=template should return mock data

	// Build search resolver
	val1 := "Template"
	resolver := &SearchResult{
		input: &model.SearchInput{Filters: []*model.SearchFilter{&model.SearchFilter{Property: "kind", Values: []*string{&val1}}}},
		pool:  mockPool,
		uids:  []string{},
		wg:    sync.WaitGroup{},
	}

	// Execute function
	r := resolver.Items()
	fmt.Println(r)

	// Verify response
	eq := reflect.DeepEqual(r, mockRow.MockValue)

	if eq {
		klog.Info("correct items")
	} else {
		t.Errorf("Incorrect Items() expected [%d] got [%d]", mockRow.MockValue, r)
	}
}
