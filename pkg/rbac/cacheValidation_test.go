// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Test_cacheValidation_namespaceAdded(t *testing.T) {
	mock_cache := Cache{
		shared: SharedData{
			namespaces:       []string{"a", "b"},
			managedClusters:  map[string]struct{}{"a": {}, "b": {}},
			disabledClusters: map[string]struct{}{"a": {}, "b": {}},
		},
	}

	mock_namespace := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": "c",
			},
		},
	}

	mock_cache.namespaceAdded(mock_namespace)

	assert.Equal(t, []string{"a", "b", "c"}, mock_cache.shared.namespaces)
	// assert.Equal(t, map[string]struct{}{"a": {}, "b": {}, "c": {}}, mock_cache.shared.disabledClusters)
	// assert.Equal(t, map[string]struct{}{"a": {}, "b": {}, "c": {}}, mock_cache.shared.managedClusters)
}

func Test_cacheValidation_namespaceDeleted(t *testing.T) {
	mock_cache := Cache{
		shared: SharedData{
			namespaces:       []string{"a", "b"},
			managedClusters:  map[string]struct{}{"a": {}, "b": {}},
			disabledClusters: map[string]struct{}{"a": {}, "b": {}},
		},
	}

	mock_namespace := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": "a",
			},
		},
	}

	mock_cache.namespaceDeleted(mock_namespace)

	assert.Equal(t, []string{"b"}, mock_cache.shared.namespaces)
	assert.Equal(t, map[string]struct{}{"b": {}}, mock_cache.shared.disabledClusters)
	assert.Equal(t, map[string]struct{}{"b": {}}, mock_cache.shared.managedClusters)
}
