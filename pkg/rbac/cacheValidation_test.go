// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

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

	assert.Equal(t, mock_cache.shared.namespaces, []string{"b"})
	assert.Equal(t, mock_cache.shared.disabledClusters, map[string]struct{}{"b": {}})
	assert.Equal(t, mock_cache.shared.managedClusters, map[string]struct{}{"b": {}})
}
