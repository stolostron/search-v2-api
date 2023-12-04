package fedresolver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"

	"k8s.io/klog/v2"
)

// mockHTTPClient is a mock implementation of the httpClient interface
type mockHTTPClient struct {
	response *http.Response
	err      error
	calls    int
	mu       sync.Mutex // Mutex to protect calls counter
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	klog.Info("*** IN DO *********")
	m.mu.Lock()
	defer m.mu.Unlock()

	// Increment the calls counter
	m.calls++
	// Create a new response with a customCloser to prevent closing the body
	resp := &http.Response{
		StatusCode: m.response.StatusCode,
		Body:       m.response.Body,
	}
	return resp, m.err
}

func TestSearchSchemaResultsErr(t *testing.T) {
	// Set your environment variables for testing
	os.Setenv("ROUTE1", "ROUTE1")
	os.Setenv("TOKEN1", "TOKEN1")
	os.Setenv("ROUTE2", "ROUTE2")
	os.Setenv("TOKEN2", "TOKEN2")

	// Create a mock response with status code 200
	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		// Add other necessary fields
		Body: io.NopCloser(bytes.NewBufferString("your mock response")),
	}

	mockClient := &mockHTTPClient{
		response: mockResponse,
		err:      nil,
	}
	searchSchemaResult := &SearchSchema{
		routeToken: getTokenAndRoute(),
		client:     mockClient,
	}

	// Call the function to be tested
	result, err := searchSchemaResult.searchSchemaResults(context.TODO())
	klog.Info(result)

	// Check if err is present
	if err == nil {
		t.Errorf("Expected searchSchemaResults to return error. Expected error, got %v", err)
	}

	// Check if the status code is 200
	if result := mockResponse.StatusCode; result != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", result)
	}
}

func TestSearchSchemaResultsHttpCallErr(t *testing.T) {
	// Set your environment variables for testing
	os.Setenv("ROUTE1", "ROUTE1")
	os.Setenv("TOKEN1", "TOKEN1")
	os.Setenv("ROUTE2", "ROUTE2")
	os.Setenv("TOKEN2", "TOKEN2")

	// Create a mock response with status code 200
	mockResponse := &http.Response{
		StatusCode: http.StatusBadRequest,
		// Add other necessary fields
		Body: io.NopCloser(bytes.NewBufferString("your mock response")),
	}

	mockClient := &mockHTTPClient{
		response: mockResponse,
		err:      nil,
	}
	searchSchemaResult := &SearchSchema{
		routeToken: getTokenAndRoute(),
		client:     mockClient,
	}

	// Call the function to be tested
	result, err := searchSchemaResult.searchSchemaResults(context.TODO())
	klog.Info(result)

	// Check if err is present
	if err == nil {
		t.Errorf("Expected searchSchemaResults to return error. Expected error, got %v", err)
	}

	// Check if the status code is 200
	if result := mockResponse.StatusCode; result == http.StatusOK {
		t.Errorf("Expected error status code, got %d", result)
	}
}

func TestSearchSchemaResultsNoRouteVarsErr(t *testing.T) {
	// Set your environment variables for testing
	os.Unsetenv("ROUTE1")
	os.Unsetenv("TOKEN1")
	os.Unsetenv("ROUTE2")
	os.Unsetenv("TOKEN2")

	os.Setenv("ROUTE", "ROUTE1")
	// Create a mock response with status code 200
	mockResponse := &http.Response{
		StatusCode: http.StatusBadRequest,
		// Add other necessary fields
		Body: io.NopCloser(bytes.NewBufferString("your mock response")),
	}

	mockClient := &mockHTTPClient{
		response: mockResponse,
		err:      nil,
	}
	searchSchemaResult := &SearchSchema{
		routeToken: getTokenAndRoute(),
		client:     mockClient,
	}

	// Call the function to be tested
	result, err := searchSchemaResult.searchSchemaResults(context.TODO())
	klog.Info(result)
	klog.Info("err: ", err)

	// Check if err is present
	if err == nil {
		t.Errorf("Expected searchSchemaResults to return error. Expected error, got %v", err)
	}

	// Check if the status code is 200
	if result := mockResponse.StatusCode; result == http.StatusOK {
		t.Errorf("Expected error status code, got %d", result)
	}
}

func TestSearchSchemaResultsHttpCallSuccess(t *testing.T) {
	// Set your environment variables for testing
	os.Setenv("ROUTE1", "ROUTE1")
	os.Setenv("TOKEN1", "TOKEN1")
	// os.Setenv("ROUTE2", "ROUTE2")
	// os.Setenv("TOKEN2", "TOKEN2")

	// Create a mock response
	mockData := schemaPayload{
		Data: struct {
			SearchSchema map[string][]string "json:\"searchSchema\""
		}{
			SearchSchema: map[string][]string{"allProperties": {"cluster", "kind", "label", "name", "namespace", "status", "node"}},
		},
	}

	// Convert the mock response to a JSON string
	jsonString, _ := json.Marshal(mockData)

	// Create a mock response with status code 200
	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		// Add other necessary fields
		// &customCloser{Reader: m.response.Body},
		Body: io.NopCloser(bytes.NewBufferString(string(jsonString))),
	}

	mockClient := &mockHTTPClient{
		response: mockResponse,
		err:      nil,
	}
	searchSchemaResult := &SearchSchema{
		routeToken: getTokenAndRoute(),
		client:     mockClient,
	}

	// Call the function to be tested
	result, err := searchSchemaResult.searchSchemaResults(context.TODO())
	klog.Info(result)

	// Check if err is present
	if err != nil {
		t.Errorf("Expected searchSchemaResults to not return error. Expected error, got %v", err)
	}

	// Check if the status code is 200
	if result := mockResponse.StatusCode; result != http.StatusOK {
		t.Errorf("Expected success status code, got %d", result)
	}
}
