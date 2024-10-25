package resolver

import "testing"

func Test_formatMapIntegers(t *testing.T) {
	policyViolationCounts := map[string]interface{}{
		"namespace/policy1": float64(2),
		"clusterpolicy2":    float64(1),
	}

	expected := "clusterpolicy2=1; namespace/policy1=2"

	rv := formatMap(policyViolationCounts)

	if rv != expected {
		t.Fatalf("Expected %s but got: %s", expected, rv)
	}
}

func Test_formatMapStrings(t *testing.T) {
	labels := map[string]interface{}{
		"hello": "world",
		"city":  "raleigh",
	}

	expected := "city=raleigh; hello=world"

	rv := formatMap(labels)

	if rv != expected {
		t.Fatalf("Expected %s but got: %s", expected, rv)
	}
}
