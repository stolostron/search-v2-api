package federated

import (
	"bytes"
	"crypto/tls"
	"io"
	"net/http"
	"testing"
)

// MockHTTPClient is a mock implementation of the HTTPClient interface.
type MockHTTPClient struct {
	Transport              http.RoundTripper
	DoFunc                 func(req *http.Request) (*http.Response, error)
	SetTLSClientConfigFunc func(config *tls.Config)
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.DoFunc(req)
}

func (m *MockHTTPClient) SetTLSClientConfig(config *tls.Config) {
	m.Transport.(*http.Transport).TLSClientConfig = config
}

func TestGetFederatedResponse(t *testing.T) {
	// Mock data
	mockRemoteService := RemoteSearchService{
		Name: "mock",
		URL:  "http://mock-service",
	}

	mockBody := []byte("mock body")

	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			// Mock HTTP response
			return &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString("mock response")),
			}, nil
		},
		// SetTLSClientConfigFunc: func(config *tls.Config) {
		// 	// Verify the TLS config if needed
		// },
	}
	// Create a FederatedRequest instance
	fedRequest := &FederatedRequest{}

	// Call the function with mock data
	fedRequest.getFederatedResponse(mockRemoteService, mockBody, mockClient)

	if len(fedRequest.Response.Errors) == 0 {
		t.Errorf("Expected errors, but got %d ", len(fedRequest.Response.Errors))

	}
}
