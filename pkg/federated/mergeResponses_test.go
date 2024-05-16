// Copyright Contributors to the Open Cluster Management project
package federated

import (
	"reflect"
	"testing"

	slices "golang.org/x/exp/slices"

	"github.com/stretchr/testify/assert"
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

func Test_mergeSearchSchema(t *testing.T) {
	d := &Data{}
	d.mergeSearchSchema([]string{"kind", "cluster"})
	shouldbeTrue := slices.Contains(d.SearchSchema.AllProperties, "managedHub")
	assert.True(t, shouldbeTrue, true, "Expected managedHub to be present in the schema. Expected true, got %t", shouldbeTrue)
}
