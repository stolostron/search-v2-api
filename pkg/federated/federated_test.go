package federated

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	config "github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/klog/v2"
)

// MockHTTPClient is a mock implementation of the HTTPClient interface
type MockHTTPClient struct {
	Transport http.Transport
	mock.Mock
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.DoFunc(req)
}

func TestHandleFederatedRequestLogReadBodyErr(t *testing.T) {

	realGetFederationConfig := getFedConfig

	defer func() { getFedConfig = realGetFederationConfig }()
	// Mock getFederationConfig function
	getFedConfig = func(ctx context.Context, request *http.Request) []RemoteSearchService {
		// Replace with your mock data
		return []RemoteSearchService{
			{
				Name: "MockService1",
				URL:  "http://mockservice1.com",
			},
			{
				Name: "MockService2",
				URL:  "http://mockservice2.com",
			},
		}
	}

	// Redirect the logger output.
	var buf bytes.Buffer
	klog.LogToStderr(false)
	klog.SetOutput(&buf)
	defer func() {
		klog.SetOutput(os.Stderr)
	}()

	// Setup HTTP request
	req := httptest.NewRequest("POST", "/federated", io.NopCloser(errorReader{}))
	// Setup HTTP response recorder
	w := httptest.NewRecorder()

	// Call the function with mock data
	HandleFederatedRequest(w, req)

	// Capture the logger output for verification.
	logMsg := buf.String()
	if !strings.Contains(logMsg, "Error reading federated request body:") {
		t.Error("Expected error reading federated request body to be logged")
	}
}

/*
func TestHandleFederatedRequestNoConfig(t *testing.T) {
	// Mock data
	mockResponseData := Data{}

	// Setup HTTP request
	req := httptest.NewRequest("POST", "/federated", bytes.NewBuffer([]byte("mock request body")))

	// Setup HTTP response recorder
	w := httptest.NewRecorder()

	// Call the function with mock data
	HandleFederatedRequest(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)
	var respBody GraphQLPayload
	err := json.NewDecoder(w.Body).Decode(&respBody)
	data := &respBody.Data

	assert.NoError(t, err)
	assert.Equal(t, &mockResponseData, data)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
}
*/

func TestHandleFederatedRequestWithConfig(t *testing.T) {
	// Mock request body
	requestBody := []byte(`{"some": "data"}`)

	// Mock HTTP request
	req := httptest.NewRequest("POST", "/federated", io.NopCloser(bytes.NewBuffer(requestBody)))
	req.Header.Set("Content-Type", "application/json")

	// Mock HTTP response
	w := httptest.NewRecorder()

	realGetFederationConfig := getFedConfig
	realGetHttpClient := httpClientGetter

	// Create a mock HTTP client
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			// Mock HTTP response
			return &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBuffer([]byte("test body"))),
			}, nil
		},
	}
	defer func() { httpClientGetter = realGetHttpClient }()

	// Set httpClientGetter to return the mock client
	httpClientGetter = func() HTTPClient {
		return mockClient
	}

	defer func() { getFedConfig = realGetFederationConfig }()
	// Mock getFederationConfig function
	getFedConfig = func(ctx context.Context, request *http.Request) []RemoteSearchService {
		// Replace with mock data
		return []RemoteSearchService{
			{
				Name: "MockService1",
				URL:  "http://mockservice1.com",
			},
			{
				Name: "MockService2",
				URL:  "http://mockservice2.com",
			},
		}
	}

	// Call the function
	HandleFederatedRequest(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var responseBody GraphQLPayload
	err := json.Unmarshal(w.Body.Bytes(), &responseBody)

	assert.Nil(t, err)
	assert.Equal(t, 2, len(responseBody.Errors))
	assert.Equal(t, 0, len(responseBody.Data.Search))
}

func TestGetFederatedResponseSuccess(t *testing.T) {
	// Create a sample response body
	payLoad := GraphQLPayload{Data: Data{
		Messages:       []string{"Welcome to Search"},
		Search:         []SearchResult{{Count: 2, Items: []map[string]interface{}{{"kind": "Pod", "ns": "ns1"}, {"kind": "Job", "ns": "ns1"}}}},
		SearchComplete: []string{"Pod", "Job"},
		SearchSchema:   &SearchSchema{AllProperties: []string{"kind", "cluster", "namespace"}},
		GraphQLSchema:  "schema",
	},
	// Errors: []error{errors.New("error fetching results from cluster1")},
	}
	responseBody, _ := json.Marshal(&payLoad)

	// Create a mock HTTP client
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			// Mock HTTP response
			return &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBuffer(responseBody)),
			}, nil
		},
	}

	// Create a sample request
	fedRequest := &FederatedRequest{} // Initialize as needed
	// Create a sample remote service
	remoteService := RemoteSearchService{} // Initialize as needed

	// Create a sample body
	receivedBody := []byte("sample body")

	// Call the function with the mock client
	fedRequest.getFederatedResponse(remoteService, receivedBody, mockClient)

	data := &fedRequest.Response.Data
	// Assertions
	assert.Empty(t, fedRequest.Response.Errors, "No errors should be recorded")
	assert.NotNil(t, data, "Data should be populated in the response")
	assert.Equal(t, 1, len(fedRequest.Response.Data.Messages))
	assert.Equal(t, 1, len(fedRequest.Response.Data.Search))
	assert.Equal(t, 2, fedRequest.Response.Data.Search[0].Count)
	assert.Equal(t, 2, len(fedRequest.Response.Data.SearchComplete))
	assert.Equal(t, 4, len(fedRequest.Response.Data.SearchSchema.AllProperties)) //managedHub is added after merge

}

func TestGetFederatedResponsePartialErrors(t *testing.T) {
	// Create a sample response body
	payLoad := GraphQLPayload{Data: Data{
		Messages:       []string{"Welcome to Search"},
		Search:         []SearchResult{{Count: 2, Items: []map[string]interface{}{{"kind": "Pod", "ns": "ns1"}, {"kind": "Job", "ns": "ns1"}}}},
		SearchComplete: []string{"Pod", "Job"},
		SearchSchema:   &SearchSchema{AllProperties: []string{"kind", "cluster", "namespace"}},
		GraphQLSchema:  "schema",
	},
		Errors: []string{fmt.Errorf("error fetching response: %s", "partial error").Error()}, // TODO: Verify partial errors are recorded
	}
	responseBody, _ := json.Marshal(&payLoad)

	// Create a mock HTTP client
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			// Mock HTTP response
			return &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBuffer(responseBody)),
			}, nil
		},
	}

	// Create a sample request
	fedRequest := &FederatedRequest{
		// Initialize as needed
	}

	// Create a sample remote service
	remoteService := RemoteSearchService{
		Name:  "TestService",
		URL:   "http://example.com",
		Token: "test-token",
	}

	// Create a sample body
	receivedBody := []byte("sample body")

	// Call the function with the mock client
	fedRequest.getFederatedResponse(remoteService, receivedBody, mockClient)

	// Assertions
	assert.Equal(t, 1, len(fedRequest.Response.Errors))

}

// TestGetFederatedResponse tests various error scenarios in getFederatedResponse.
func TestGetFederatedResponseErrors(t *testing.T) {

	testCases := []struct {
		name           string
		mockClientFunc func() *MockHTTPClient
		remoteService  RemoteSearchService
		receivedBody   []byte
		expectedError  string
	}{
		{
			name: "Error creating federated request",
			mockClientFunc: func() *MockHTTPClient {
				return &MockHTTPClient{
					DoFunc: func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							Status:     "Bad Request",
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewBuffer([]byte("test body"))),
						}, nil
					},
				}
			},
			remoteService: RemoteSearchService{
				Name: "TestService",
				URL:  "%invalid url%", // Simulate an unsuccessful request
			},
			expectedError: "error creating federated request",
		},
		{
			name: "Error sending federated request",
			mockClientFunc: func() *MockHTTPClient {
				return &MockHTTPClient{
					DoFunc: func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							Status:     "Bad Request",
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewBuffer([]byte("test body"))),
						}, errors.New("error in request") // Simulate an unsuccessful request
					},
				}
			},
			remoteService: RemoteSearchService{
				Name: "TestService",
				URL:  "http://example.com",
			},
			expectedError: "error sending federated request",
		},
		{
			name: "Error reading federated request",
			mockClientFunc: func() *MockHTTPClient {
				return &MockHTTPClient{
					DoFunc: func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							Status:     "Bad Request",
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(errorReader{}), // Simulate an unsuccessful request
						}, nil
					},
				}
			},
			remoteService: RemoteSearchService{
				Name: "TestService",
				URL:  "http://example.com",
			},
			// receivedBody:  []byte("error body"),
			expectedError: "error reading federated response body",
		},
		{
			name: "Error parsing federated response",
			mockClientFunc: func() *MockHTTPClient {
				return &MockHTTPClient{
					DoFunc: func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							Status:     "Bad Request",
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewBuffer([]byte("wrong response body"))),
						}, nil // Simulate an unsuccessful request
					},
				}
			},
			remoteService: RemoteSearchService{
				Name: "TestService",
				URL:  "http://example.com",
			},
			expectedError: "error parsing response",
		},
		// {
		// 	name: "Partial error in response",
		// 	mockClientFunc: func() *MockHTTPClient {
		// 		return &MockHTTPClient{
		// 			DoFunc: func(req *http.Request) (*http.Response, error) {
		// 				return &http.Response{
		// 					Status:     "Bad Request",
		// 					StatusCode: http.StatusOK,
		// 					Body:       io.NopCloser(bytes.NewBuffer([]byte(responseBody))),
		// 				}, nil // Simulate an unsuccessful request
		// 			},
		// 		}
		// 	},
		// 	remoteService: RemoteSearchService{
		// 		Name: "TestService",
		// 		URL:  "http://example.com",
		// 	},
		// 	expectedError: "error partial response",
		// },
		// Add more test cases for other error scenarios
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := tc.mockClientFunc()

			// Create a FederatedRequest instance
			fedRequest := &FederatedRequest{
				Response: GraphQLPayload{},
			}

			// Call the function with mock data
			fedRequest.getFederatedResponse(tc.remoteService, tc.receivedBody, mockClient)

			// Assert that the expected error is present in the response errors
			assert.Contains(t, fedRequest.Response.Errors[0], tc.expectedError)
		})
	}
}

// errorReader is a custom reader that always returns an error.
type errorReader struct{}

func (er errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("simulated error reading response body")
}

func (er errorReader) Close(p []byte) (n int, err error) {
	return 0, errors.New("simulated error reading response body")
}

func TestManagedHubFederatedResponseSuccess(t *testing.T) {
	callNum := 0

	config.Cfg.Federation.GlobalHubName = "test-hub-a"
	// Mock fedConfig
	cachedFedConfig = fedConfigCache{
		lastUpdated: time.Now(),
		fedConfig: []RemoteSearchService{{Name: "test-hub-a",
			URL:   "https://api.mockHubUrl.com:6443",
			Token: "mockToken",
		}},
	}
	// Mock data
	mockResponseData := Data{
		Search:        []SearchResult{{Count: 2, Items: []map[string]interface{}{{"kind": "Pod", "managedHub": "test-hub-a", "ns": "ns1"}, {"kind": "Job", "managedHub": "test-hub-a", "ns": "ns1"}}}},
		GraphQLSchema: "schema",
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	realGetHttpClient := httpClientGetter

	// Create a sample response body
	realPayLoad := GraphQLPayload{Data: Data{
		Search:        []SearchResult{{Count: 2, Items: []map[string]interface{}{{"kind": "Pod", "ns": "ns1"}, {"kind": "Job", "ns": "ns1"}}}},
		GraphQLSchema: "schema",
	},
		Errors: nil,
	}
	realResponseBody, _ := json.Marshal(&realPayLoad)

	emptyPayLoad := GraphQLPayload{Data: Data{
		Search:        []SearchResult{{Count: 0, Items: []map[string]interface{}{}}},
		GraphQLSchema: "schema",
	},
		Errors: nil,
	}
	emptyResponseBody, _ := json.Marshal(&emptyPayLoad)

	// Create a mock HTTP client
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			callNum++
			if callNum == 1 { // for test-hub-a managed cluster
				// Mock HTTP response
				return &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBuffer(realResponseBody)),
				}, nil
			} else { // for global-hub cluster
				return &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBuffer(emptyResponseBody)),
				}, nil
			}
		},
	}
	defer func() { httpClientGetter = realGetHttpClient }()

	// Set httpClientGetter to return the mock client
	httpClientGetter = func() HTTPClient {
		return mockClient
	}

	receivedBody := []byte(`{"query":"query{\n  search(\n  input: [\n  {filters:[{property:\"kind\",values:[\"*\"]},\n    {property:\"managedHub\",values:[\"test-hub-a\"]}\n  ],limit:1000}]\n\t\t\t   ) {\n\t\t\t     count\n    items\n\t\t\t   }\n\t}"}`)

	// Setup HTTP request
	req := httptest.NewRequest("POST", "/federated", bytes.NewBuffer(receivedBody))

	// Setup HTTP response recorder
	w := httptest.NewRecorder()

	// Call the function with mock data
	HandleFederatedRequest(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)
	var respBody = GraphQLPayload{}

	err := json.NewDecoder(w.Body).Decode(&respBody)
	data := &respBody.Data

	assert.NoError(t, err)
	assert.Equal(t, &mockResponseData, data)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
}

// TestManagedHubFilterPath1_SearchSchema tests extracting managedHub values from path 1:
// variables.query.filters (used in searchSchema queries) - covers lines 58-62
func TestManagedHubFilterPath1_SearchSchema(t *testing.T) {
	realGetFederationConfig := getFedConfig
	realGetHttpClient := httpClientGetter

	defer func() {
		getFedConfig = realGetFederationConfig
		httpClientGetter = realGetHttpClient
	}()

	callCount := 0

	// Mock getFederationConfig to return two services
	getFedConfig = func(ctx context.Context, request *http.Request) []RemoteSearchService {
		return []RemoteSearchService{
			{Name: "hub-1", URL: "http://hub1.com", Token: "token1"},
			{Name: "hub-2", URL: "http://hub2.com", Token: "token2"},
		}
	}

	// Create a sample response body
	payLoad := GraphQLPayload{
		Data: Data{
			SearchSchema: &SearchSchema{AllProperties: []string{"kind", "cluster"}},
		},
		Errors: nil,
	}
	responseBody, _ := json.Marshal(&payLoad)

	// Create a mock HTTP client
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			return &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBuffer(responseBody)),
			}, nil
		},
	}

	httpClientGetter = func() HTTPClient {
		return mockClient
	}

	// Request body with managedHub filter in path 1: variables.query.filters (searchSchema)
	receivedBody := []byte(`{
		"operationName": "searchSchema",
		"variables": {
			"query": {
				"filters": [
					{"property": "kind", "values": ["Pod"]},
					{"property": "managedHub", "values": ["hub-1"]}
				]
			}
		},
		"query": "query searchSchema($query: SearchInput) { searchSchema(query: $query) }"
	}`)

	// Setup HTTP request
	req := httptest.NewRequest("POST", "/federated", bytes.NewBuffer(receivedBody))
	w := httptest.NewRecorder()

	// Call the function
	HandleFederatedRequest(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)
	// Only hub-1 should be called since managedHub filter specifies only hub-1
	assert.Equal(t, 1, callCount, "Expected only 1 remote service to be called")
}

// TestEmptyManagedHubFilter tests that when no managedHub filter is provided,
// all remote services are called - covers lines 97-109 (empty check)
func TestEmptyManagedHubFilter(t *testing.T) {
	realGetFederationConfig := getFedConfig
	realGetHttpClient := httpClientGetter

	defer func() {
		getFedConfig = realGetFederationConfig
		httpClientGetter = realGetHttpClient
	}()

	callCount := 0

	// Mock getFederationConfig to return three services
	getFedConfig = func(ctx context.Context, request *http.Request) []RemoteSearchService {
		return []RemoteSearchService{
			{Name: "hub-1", URL: "http://hub1.com", Token: "token1"},
			{Name: "hub-2", URL: "http://hub2.com", Token: "token2"},
			{Name: "hub-3", URL: "http://hub3.com", Token: "token3"},
		}
	}

	// Create a sample response body
	payLoad := GraphQLPayload{
		Data: Data{
			Search: []SearchResult{{Count: 0, Items: []map[string]interface{}{}}},
		},
		Errors: nil,
	}
	responseBody, _ := json.Marshal(&payLoad)

	// Create a mock HTTP client
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			return &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBuffer(responseBody)),
			}, nil
		},
	}

	httpClientGetter = func() HTTPClient {
		return mockClient
	}

	// Request body WITHOUT managedHub filter
	receivedBody := []byte(`{
		"operationName": "searchResult",
		"variables": {
			"input": [{
				"filters": [
					{"property": "kind", "values": ["Pod"]}
				],
				"limit": 1000
			}]
		},
		"query": "query searchResult { search(input: $input) { count items } }"
	}`)

	// Setup HTTP request
	req := httptest.NewRequest("POST", "/federated", bytes.NewBuffer(receivedBody))
	w := httptest.NewRecorder()

	// Call the function
	HandleFederatedRequest(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)
	// All 3 services should be called since there's no managedHub filter
	assert.Equal(t, 3, callCount, "Expected all 3 remote services to be called when no managedHub filter is provided")
}

// TestMultipleManagedHubValues tests filtering with multiple managedHub values
// Covers lines 76-88 (extracting multiple values) and lines 99-103 (filtering loop)
func TestMultipleManagedHubValues(t *testing.T) {
	realGetFederationConfig := getFedConfig
	realGetHttpClient := httpClientGetter

	defer func() {
		getFedConfig = realGetFederationConfig
		httpClientGetter = realGetHttpClient
	}()

	callCount := 0
	calledServices := []string{}

	// Mock getFederationConfig to return four services
	getFedConfig = func(ctx context.Context, request *http.Request) []RemoteSearchService {
		return []RemoteSearchService{
			{Name: "hub-1", URL: "http://hub1.com", Token: "token1"},
			{Name: "hub-2", URL: "http://hub2.com", Token: "token2"},
			{Name: "hub-3", URL: "http://hub3.com", Token: "token3"},
			{Name: "hub-4", URL: "http://hub4.com", Token: "token4"},
		}
	}

	// Create a sample response body
	payLoad := GraphQLPayload{
		Data: Data{
			Search: []SearchResult{{Count: 0, Items: []map[string]interface{}{}}},
		},
		Errors: nil,
	}
	responseBody, _ := json.Marshal(&payLoad)

	// Create a mock HTTP client
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			// Track which service is being called by inspecting the URL
			if strings.Contains(req.URL.String(), "hub1.com") {
				calledServices = append(calledServices, "hub-1")
			} else if strings.Contains(req.URL.String(), "hub2.com") {
				calledServices = append(calledServices, "hub-2")
			} else if strings.Contains(req.URL.String(), "hub3.com") {
				calledServices = append(calledServices, "hub-3")
			} else if strings.Contains(req.URL.String(), "hub4.com") {
				calledServices = append(calledServices, "hub-4")
			}
			return &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBuffer(responseBody)),
			}, nil
		},
	}

	httpClientGetter = func() HTTPClient {
		return mockClient
	}

	// Request body with MULTIPLE managedHub values (hub-1 and hub-3)
	receivedBody := []byte(`{
		"operationName": "searchResult",
		"variables": {
			"input": [{
				"filters": [
					{"property": "kind", "values": ["Pod"]},
					{"property": "managedHub", "values": ["hub-1", "hub-3"]}
				],
				"limit": 1000
			}]
		},
		"query": "query searchResult { search(input: $input) { count items } }"
	}`)

	// Setup HTTP request
	req := httptest.NewRequest("POST", "/federated", bytes.NewBuffer(receivedBody))
	w := httptest.NewRecorder()

	// Call the function
	HandleFederatedRequest(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)
	// Only hub-1 and hub-3 should be called (2 services)
	assert.Equal(t, 2, callCount, "Expected 2 remote services to be called for multiple managedHub values")
	// Verify the correct services were called
	assert.Contains(t, calledServices, "hub-1", "hub-1 should be called")
	assert.Contains(t, calledServices, "hub-3", "hub-3 should be called")
	assert.NotContains(t, calledServices, "hub-2", "hub-2 should NOT be called")
	assert.NotContains(t, calledServices, "hub-4", "hub-4 should NOT be called")
}

// TestRemoteServiceSkipped tests that services not in the managedHub filter are skipped
// Covers lines 105-108 (skip logic and logging)
func TestRemoteServiceSkipped(t *testing.T) {
	realGetFederationConfig := getFedConfig
	realGetHttpClient := httpClientGetter

	defer func() {
		getFedConfig = realGetFederationConfig
		httpClientGetter = realGetHttpClient
	}()

	callCount := 0
	calledServices := []string{}

	// Mock getFederationConfig to return three services
	getFedConfig = func(ctx context.Context, request *http.Request) []RemoteSearchService {
		return []RemoteSearchService{
			{Name: "target-hub", URL: "http://targethub.com", Token: "token1"},
			{Name: "skipped-hub-1", URL: "http://skippedhub1.com", Token: "token2"},
			{Name: "skipped-hub-2", URL: "http://skippedhub2.com", Token: "token3"},
		}
	}

	// Create a sample response body
	payLoad := GraphQLPayload{
		Data: Data{
			Search: []SearchResult{{Count: 0, Items: []map[string]interface{}{}}},
		},
		Errors: nil,
	}
	responseBody, _ := json.Marshal(&payLoad)

	// Create a mock HTTP client
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			// Track which service is being called by inspecting the URL
			if strings.Contains(req.URL.String(), "targethub.com") {
				calledServices = append(calledServices, "target-hub")
			} else if strings.Contains(req.URL.String(), "skippedhub1.com") {
				calledServices = append(calledServices, "skipped-hub-1")
			} else if strings.Contains(req.URL.String(), "skippedhub2.com") {
				calledServices = append(calledServices, "skipped-hub-2")
			}
			return &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBuffer(responseBody)),
			}, nil
		},
	}

	httpClientGetter = func() HTTPClient {
		return mockClient
	}

	// Request body with managedHub filter that only includes "target-hub"
	receivedBody := []byte(`{
		"operationName": "searchResult",
		"variables": {
			"input": [{
				"filters": [
					{"property": "kind", "values": ["Pod"]},
					{"property": "managedHub", "values": ["target-hub"]}
				],
				"limit": 1000
			}]
		},
		"query": "query searchResult { search(input: $input) { count items } }"
	}`)

	// Setup HTTP request
	req := httptest.NewRequest("POST", "/federated", bytes.NewBuffer(receivedBody))
	w := httptest.NewRecorder()

	// Call the function
	HandleFederatedRequest(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)
	// Only target-hub should be called, other services should be skipped
	assert.Equal(t, 1, callCount, "Expected only 1 remote service (target-hub) to be called")
	assert.Contains(t, calledServices, "target-hub", "target-hub should be called")
	assert.NotContains(t, calledServices, "skipped-hub-1", "skipped-hub-1 should NOT be called")
	assert.NotContains(t, calledServices, "skipped-hub-2", "skipped-hub-2 should NOT be called")
}

// TestInvalidJSONUnmarshaling tests that when JSON unmarshaling fails,
// an error is logged but execution continues with all services being called
// Covers lines 50-52 (error handling for unmarshaling)
func TestInvalidJSONUnmarshaling(t *testing.T) {
	realGetFederationConfig := getFedConfig
	realGetHttpClient := httpClientGetter

	defer func() {
		getFedConfig = realGetFederationConfig
		httpClientGetter = realGetHttpClient
	}()

	// Redirect the logger output to capture error messages
	var buf bytes.Buffer
	klog.LogToStderr(false)
	klog.SetOutput(&buf)
	defer func() {
		klog.SetOutput(os.Stderr)
	}()

	callCount := 0

	// Mock getFederationConfig to return two services
	getFedConfig = func(ctx context.Context, request *http.Request) []RemoteSearchService {
		return []RemoteSearchService{
			{Name: "hub-1", URL: "http://hub1.com", Token: "token1"},
			{Name: "hub-2", URL: "http://hub2.com", Token: "token2"},
		}
	}

	// Create a sample response body
	payLoad := GraphQLPayload{
		Data: Data{
			Search: []SearchResult{{Count: 0, Items: []map[string]interface{}{}}},
		},
		Errors: nil,
	}
	responseBody, _ := json.Marshal(&payLoad)

	// Create a mock HTTP client
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			return &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBuffer(responseBody)),
			}, nil
		},
	}

	httpClientGetter = func() HTTPClient {
		return mockClient
	}

	// Invalid JSON body (missing closing brace)
	receivedBody := []byte(`{"operationName": "searchResult", "variables": {`)

	// Setup HTTP request
	req := httptest.NewRequest("POST", "/federated", bytes.NewBuffer(receivedBody))
	w := httptest.NewRecorder()

	// Call the function
	HandleFederatedRequest(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)
	// All services should still be called since unmarshaling failed and managedHubValues is empty
	assert.Equal(t, 2, callCount, "Expected all services to be called when JSON unmarshaling fails")

	// Verify error message is logged
	logMsg := buf.String()
	assert.Contains(t, logMsg, "Error unmarshaling federated request body", "Should log unmarshaling error")
}

// TestModifyRequestBodyForVersion_NonV213 tests that non-2.13 versions don't get modified
func TestModifyRequestBodyForVersion_NonV213(t *testing.T) {
	remoteService := RemoteSearchService{
		Name:    "hub-1",
		Version: "2.14.0",
	}

	requestBody := []byte(`{
		"operationName": "searchSchema",
		"variables": {"query": {"filters": [{"property": "kind", "values": ["Pod"]}]}},
		"query": "query searchSchema($query: SearchInput) { searchSchema(query: $query) }"
	}`)

	result := modifyRequestBodyForVersion(remoteService, requestBody)

	// Should return the original body unchanged
	assert.Equal(t, requestBody, result, "Non-2.13 versions should not be modified")
}

// TestModifyRequestBodyForVersion_EmptyVersion tests that empty version doesn't get modified
func TestModifyRequestBodyForVersion_EmptyVersion(t *testing.T) {
	remoteService := RemoteSearchService{
		Name:    "hub-1",
		Version: "",
	}

	requestBody := []byte(`{
		"operationName": "searchSchema",
		"variables": {"query": {"filters": [{"property": "kind", "values": ["Pod"]}]}},
		"query": "query searchSchema($query: SearchInput) { searchSchema(query: $query) }"
	}`)

	result := modifyRequestBodyForVersion(remoteService, requestBody)

	// Should return the original body unchanged
	assert.Equal(t, requestBody, result, "Empty version should not be modified")
}

// TestModifyRequestBodyForVersion_V213_NotSearchSchema tests that 2.13 version with non-searchSchema query doesn't get modified
func TestModifyRequestBodyForVersion_V213_NotSearchSchema(t *testing.T) {
	remoteService := RemoteSearchService{
		Name:    "hub-1",
		Version: "2.13.0",
	}

	requestBody := []byte(`{
		"operationName": "searchResult",
		"variables": {"input": [{"filters": [{"property": "kind", "values": ["Pod"]}]}]},
		"query": "query searchResult { search(input: $input) { count items } }"
	}`)

	result := modifyRequestBodyForVersion(remoteService, requestBody)

	// Should return the original body unchanged for non-searchSchema queries
	assert.Equal(t, requestBody, result, "Non-searchSchema queries should not be modified even for 2.13")
}

// TestModifyRequestBodyForVersion_V213_SearchSchema tests that 2.13 version with searchSchema query gets modified
func TestModifyRequestBodyForVersion_V213_SearchSchema(t *testing.T) {
	remoteService := RemoteSearchService{
		Name:    "hub-1",
		Version: "2.13.0",
	}

	requestBody := []byte(`{
		"operationName": "searchSchema",
		"variables": {"query": {"filters": [{"property": "kind", "values": ["Pod"]}]}},
		"query": "query searchSchema($query: SearchInput) { searchSchema(query: $query) }"
	}`)

	result := modifyRequestBodyForVersion(remoteService, requestBody)

	// Should modify the query to remove $query parameter
	assert.NotEqual(t, requestBody, result, "2.13 searchSchema query should be modified")

	// Parse the result to verify the modification
	var resultMap map[string]interface{}
	err := json.Unmarshal(result, &resultMap)
	assert.NoError(t, err, "Modified body should be valid JSON")

	// Check that the query has been modified
	queryStr, ok := resultMap["query"].(string)
	assert.True(t, ok, "Query should be a string")
	assert.NotContains(t, queryStr, "$query: SearchInput", "Modified query should not contain parameter definition")
	assert.NotContains(t, queryStr, "query: $query", "Modified query should not contain parameter usage")
	assert.Contains(t, queryStr, "searchSchema", "Modified query should still contain searchSchema")
}

// TestModifyRequestBodyForVersion_V213_NoQueryParameter tests 2.13 searchSchema without $query parameter
func TestModifyRequestBodyForVersion_V213_NoQueryParameter(t *testing.T) {
	remoteService := RemoteSearchService{
		Name:    "hub-1",
		Version: "2.13.0",
	}

	// Request body without $query parameter (already in simple format)
	requestBody := []byte(`{
		"operationName": "searchSchema",
		"variables": {},
		"query": "query searchSchema { searchSchema }"
	}`)

	result := modifyRequestBodyForVersion(remoteService, requestBody)

	// Should return the body unchanged since there's no $query to remove
	var originalMap, resultMap map[string]interface{}
	assert.Nil(t, json.Unmarshal(requestBody, &originalMap))
	assert.Nil(t, json.Unmarshal(result, &resultMap))

	assert.Equal(t, originalMap["query"], resultMap["query"], "Query without $query parameter should remain unchanged")
}

// TestModifyRequestBodyForVersion_InvalidJSON tests that invalid JSON is handled gracefully
func TestModifyRequestBodyForVersion_InvalidJSON(t *testing.T) {
	// Redirect the logger output to capture error messages
	var buf bytes.Buffer
	klog.LogToStderr(false)
	klog.SetOutput(&buf)
	defer func() {
		klog.SetOutput(os.Stderr)
	}()

	remoteService := RemoteSearchService{
		Name:    "hub-1",
		Version: "2.13.0",
	}

	// Invalid JSON
	requestBody := []byte(`{"operationName": "searchSchema", "query": `)

	result := modifyRequestBodyForVersion(remoteService, requestBody)

	// Should return the original body unchanged when JSON is invalid
	assert.Equal(t, requestBody, result, "Invalid JSON should be returned unchanged")

	// Verify error was logged
	logMsg := buf.String()
	assert.Contains(t, logMsg, "Error unmarshaling request body for version modification", "Should log unmarshaling error")
}
