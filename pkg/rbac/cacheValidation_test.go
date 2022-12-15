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

func Test_cacheValidation_StartBackgroundValidation(t *testing.T) {
	testScheme := scheme.Scheme
	mockns := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
	}
	mock_cache := Cache{
		shared: SharedData{
			namespaces:       []string{"a", "b"},
			managedClusters:  map[string]struct{}{"a": {}, "b": {}},
			disabledClusters: map[string]struct{}{"a": {}, "b": {}},
			dynamicClient:    fakedynclient.NewSimpleDynamicClient(testScheme, mockns),
		},
		users: map[string]*UserDataCache{"usr1": &UserDataCache{}},
	}

	ctx := context.Background()
	mock_cache.StartBackgroundValidation(ctx)
}

func Test_cacheValidation_namespaceAdded(t *testing.T) {
	testScheme := scheme.Scheme
	mockns := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
	}
	mock_cache := Cache{
		shared: SharedData{
			namespaces:       []string{"a", "b"},
			managedClusters:  map[string]struct{}{"a": {}, "b": {}},
			disabledClusters: map[string]struct{}{"a": {}, "b": {}},
			dynamicClient:    fakedynclient.NewSimpleDynamicClient(testScheme, mockns),
		},
		users: map[string]*UserDataCache{
			"usr1": &UserDataCache{UserData: UserData{NsResources: map[string][]Resource{}}},
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
}

func Test_cacheValidation_namespaceDeleted(t *testing.T) {
	mock_cache := Cache{
		shared: SharedData{
			namespaces:       []string{"a", "b"},
			managedClusters:  map[string]struct{}{"a": {}, "b": {}},
			disabledClusters: map[string]struct{}{"a": {}, "b": {}},
		},
		users: map[string]*UserDataCache{"usr1": &UserDataCache{}},
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
