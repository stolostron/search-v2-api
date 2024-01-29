package federated

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/klog/v2"
)

// MockHTTPClient is a mock implementation of the HTTPClient interface
type MockHTTPClient struct {
	Transport http.Transport
	mock.Mock
	DoFunc                 func(req *http.Request) (*http.Response, error)
	SetTLSClientConfigFunc func(config *tls.Config)
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.DoFunc(req)
}

func (m *MockHTTPClient) SetTLSClientConfig(config *tls.Config) {
	m.Transport.TLSClientConfig = config
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
	if !strings.Contains(logMsg, "Error reading request body:") {
		t.Error("Expected error reading request body to be logged")
	}
}

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
		SetTLSClientConfigFunc: func(config *tls.Config) {
		},
	}
	defer func() { httpClientGetter = realGetHttpClient }()

	// Set httpClientGetter to return the mock client
	httpClientGetter = func(remoteService RemoteSearchService) HTTPClient {
		return mockClient
	}

	defer func() { getFedConfig = realGetFederationConfig }()
	// Mock getFederationConfig function
	getFedConfig = func(ctx context.Context, request *http.Request) []RemoteSearchService {
		// Replace with mock data
		return []RemoteSearchService{
			{
				Name:    "MockService1",
				URL:     "http://mockservice1.com",
				TLSCert: "tlscert1",
				TLSKey:  "tlskey1",
			},
			{
				Name:    "MockService2",
				URL:     "http://mockservice2.com",
				TLSCert: "tlscert2",
				TLSKey:  "tlskey2",
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
		SetTLSClientConfigFunc: func(config *tls.Config) {
			// Verify the TLS config if needed
		},
	}

	// Create a sample request
	fedRequest := &FederatedRequest{} // Initialize as needed
	// Create a sample remote service
	remoteService := RemoteSearchService{} // Initialize as needed

	// Set up an expectation for SetTLSClientConfig
	expectedTLSConfig := &tls.Config{MinVersion: tls.VersionTLS13}
	mockClient.On("SetTLSClientConfig", expectedTLSConfig)

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
	assert.Equal(t, 3, len(fedRequest.Response.Data.SearchSchema.AllProperties))

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
		Name:    "TestService",
		URL:     "http://example.com",
		Token:   "test-token",
		TLSCert: "cert-xxx",
		TLSKey:  "key-xxx",
	}
	// Set up an expectation for SetTLSClientConfig
	expectedTLSConfig := &tls.Config{MinVersion: tls.VersionTLS13}
	mockClient.On("SetTLSClientConfig", expectedTLSConfig)

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
