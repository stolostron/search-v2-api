// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	fakedynclient "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/scheme"
)

func initMockCache() Cache {
	testScheme := scheme.Scheme
	mockns := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
	}
	return Cache{
		shared: SharedData{
			namespaces:       []string{"a", "b"},
			managedClusters:  map[string]struct{}{"a": {}, "b": {}},
			disabledClusters: map[string]struct{}{"a": {}, "b": {}},
			dynamicClient:    fakedynclient.NewSimpleDynamicClient(testScheme, mockns),
		},
		users: map[string]*UserDataCache{
			"usr1": {UserData: UserData{NsResources: map[string][]Resource{}}},
		},
	}
}

func Test_cacheValidation_StartBackgroundValidation(t *testing.T) {
	mock_cache := initMockCache()

	ctx := context.Background()
	mock_cache.StartBackgroundValidation(ctx)
}

func Test_cacheValidation_namespaceAdded(t *testing.T) {
	mock_cache := initMockCache()
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
}

func Test_cacheValidation_namespaceDeleted(t *testing.T) {
	mock_cache := initMockCache()
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
}

func Test_cacheValidation_ManagedClusterAdded(t *testing.T) {
	mock_cache := initMockCache()
	mock_managedCluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "c",
			},
		},
	}

	mock_cache.managedClusterAdded(mock_managedCluster)
	assert.Equal(t, map[string]struct{}{"a": {}, "b": {}, "c": {}}, mock_cache.shared.managedClusters)
}

func Test_cacheValidation_managedClusterDeleted(t *testing.T) {
	mock_cache := initMockCache()

	mock_managedCluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "a",
			},
		},
	}

	mock_cache.managedClusterDeleted(mock_managedCluster)
	assert.Equal(t, map[string]struct{}{"b": {}}, mock_cache.shared.managedClusters)
}
