package federated

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
		Errors: []error{fmt.Errorf("error fetching response: %s", "partial error")}, // TODO: Verify partial errors are recorded
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
			assert.Contains(t, fedRequest.Response.Errors[0].Error(), tc.expectedError)
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
