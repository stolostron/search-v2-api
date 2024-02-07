// Copyright Contributors to the Open Cluster Management project
package federated

import (
	"reflect"
	"testing"
)

func TestAppendRelatedResults(t *testing.T) {
	mergedItems := []SearchRelatedResult{
		{Kind: "kind1", Count: 2, Items: []map[string]interface{}{{"name": "result1"}}},
		{Kind: "kind2", Count: 3, Items: []map[string]interface{}{{"name": "result2"}}},
	}

	newItems := []SearchRelatedResult{
		{Kind: "kind1", Count: 1, Items: []map[string]interface{}{{"name": "result3"}}},
		{Kind: "kind3", Count: 4, Items: []map[string]interface{}{{"name": "result4"}}},
	}

	expected := []SearchRelatedResult{
		{Kind: "kind1", Count: 3, Items: []map[string]interface{}{{"name": "result1"}, {"name": "result3"}}},
		{Kind: "kind2", Count: 3, Items: []map[string]interface{}{{"name": "result2"}}},
		{Kind: "kind3", Count: 4, Items: []map[string]interface{}{{"name": "result4"}}},
	}

	d := &Data{}
	result := d.appendRelatedResults(mergedItems, newItems)

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Unexpected result. Expected: %v, Got: %v", expected, result)
	}
}
