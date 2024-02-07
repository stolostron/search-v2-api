import (
	"reflect"
	"testing"
)

func TestAppendRelatedResults(t *testing.T) {
	mergedItems := []SearchRelatedResult{
		{Kind: "kind1", Count: 2, Items: []string{"item1"}},
		{Kind: "kind2", Count: 3, Items: []string{"item2"}},
	}

	newItems := []SearchRelatedResult{
		{Kind: "kind1", Count: 1, Items: []string{"item3"}},
		{Kind: "kind3", Count: 4, Items: []string{"item4"}},
	}

	expected := []SearchRelatedResult{
		{Kind: "kind1", Count: 3, Items: []string{"item1", "item3"}},
		{Kind: "kind2", Count: 3, Items: []string{"item2"}},
		{Kind: "kind3", Count: 4, Items: []string{"item4"}},
	}

	d := &Data{}
	result := d.appendRelatedResults(mergedItems, newItems)

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Unexpected result. Expected: %v, Got: %v", expected, result)
	}
}