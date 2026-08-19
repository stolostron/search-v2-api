package federated

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	fakedynclient "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestGetFederatedConfig_fromCache(t *testing.T) {
	cachedFedConfig = fedConfigCache{
		lastUpdated: time.Now(),
		fedConfig: []RemoteSearchService{
			{
				Name:  "mock-cache-name",
				URL:   "https://mock-cache-url",
				Token: "mock-cache-token",
			},
		},
	}

	mockRequest := &http.Request{}
	ctx := context.Background()
	result := getFederationConfig(ctx, mockRequest)

	assert.Equal(t, 1, len(result))
	assert.Equal(t, "mock-cache-name", result[0].Name)
	assert.Equal(t, "https://mock-cache-url", result[0].URL)
	assert.Equal(t, "mock-cache-token", result[0].Token)
}

func TestGetLocalSearchApiConfig(t *testing.T) {
	mockRequest := &http.Request{
		Header: map[string][]string{
			"Authorization": {"Bearer mock-token"},
		},
	}
	result := getLocalSearchApiConfig(context.Background(), mockRequest)

	assert.Equal(t, result.Name, "global-hub")
	assert.Equal(t, result.URL, "https://search-search-api.open-cluster-management.svc:4010/searchapi/graphql")
	assert.Equal(t, result.Token, "mock-token")
}

// Builds a fake Kubernetes client to mock data in tests.
func buildFakeKubernetesClient() kubernetes.Interface {
	fakeClient := fake.NewSimpleClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "search-global",
				Namespace: "mock-managed-cluster",
			},
			Data: map[string][]byte{
				"token": []byte("mock-token"),
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "search-ca-crt",
				Namespace: "open-cluster-management",
			},
			Data: map[string]string{
				"service-ca-crt": "mock-token",
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kube-root-ca.crt",
				Namespace: "openshift-service-ca",
			},
			Data: map[string]string{
				"ca.crt": "mock-ca-bundle",
			},
		},
	)
	return fakeClient
}

// Builds a fake dynamic client to mock data in tests.
func buildFakeDynamicClient() dynamic.Interface {
	gvrToListKind := map[schema.GroupVersionResource]string{
		{Group: "route.openshift.io", Version: "v1", Resource: "routes"}:                          "RoutesList",
		{Group: "cluster.open-cluster-management.io", Version: "v1", Resource: "managedclusters"}: "ManagedClustersList",
	}
	fakeClient := fakedynclient.NewSimpleDynamicClientWithCustomListKinds(
		scheme.Scheme, gvrToListKind,
		[]runtime.Object{
			&unstructured.UnstructuredList{
				Object: map[string]interface{}{
					"apiVersion": "route.openshift.io/v1",
					"kind":       "Route",
				},
				Items: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"apiVersion": "route.openshift.io/v1",
							"kind":       "Route",
							"metadata": map[string]interface{}{
								"name":      "cluster-proxy-user",
								"namespace": "open-cluster-management",
							},
							"spec": map[string]interface{}{
								"host": "mock-cluster-proxy-route",
							},
						},
					},
				},
			},
			&unstructured.UnstructuredList{
				Object: map[string]interface{}{
					"apiVersion": "cluster.open-cluster-management.io/v1",
					"kind":       "ManagedCluster",
				},
				Items: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"apiVersion": "cluster.open-cluster-management.io/v1",
							"kind":       "ManagedCluster",
							"metadata": map[string]interface{}{
								"name": "mock-managed-cluster",
							},
							"status": map[string]interface{}{
								"clusterClaims": []interface{}{
									map[string]interface{}{
										"name":  "hub.open-cluster-management.io",
										"value": "Installed",
									},
								},
							},
						},
					},
				},
			},
		}...)
	return fakeClient
}

// Builds a fake HTTP client to mock data in tests.
func buildFakeHttpClient() *MockHTTPClient {
	return &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			// Mock HTTP response
			return &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Body: io.NopCloser(bytes.NewBuffer([]byte(
					`{
						"data": {
							"searchResult": [
								{"items":[{"cluster": "mock-managed-cluster", "namespace": "mock-namespace"}]}
							]
						}
					}`))),
			}, nil
		},
	}
}

func TestGetFederationConfigFromSecret(t *testing.T) {
	// Mock Kubernetes client data.
	getKubeClient = func() kubernetes.Interface {
		return buildFakeKubernetesClient()
	}
	// Mock dynamic client data.
	getDynamicClient = func() dynamic.Interface {
		return buildFakeDynamicClient()
	}
	// Mock HTTP client data.
	httpClientGetter = func() HTTPClient {
		return buildFakeHttpClient()
	}

	mockRequest := &http.Request{
		Header: map[string][]string{
			"Authorization": {"Bearer mock-token"},
		},
	}
	result := getFederationConfigFromSecret(context.Background(), mockRequest)

	// Validate the result.
	assert.Equal(t, 2, len(result))
	assert.Equal(t, "global-hub", result[0].Name)
	assert.Equal(t, "https://search-search-api.open-cluster-management.svc:4010/searchapi/graphql", result[0].URL)
	assert.Equal(t, "mock-token", result[0].Token)

	assert.Equal(t, "mock-managed-cluster", result[1].Name)
	assert.Equal(t, "https://mock-cluster-proxy-route/mock-managed-cluster/api/v1/namespaces/mock-namespace/services/search-search-api:4010/proxy-service/searchapi/graphql", result[1].URL)
	assert.Equal(t, "mock-token", result[1].Token)
}
