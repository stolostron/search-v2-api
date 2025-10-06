// Copyright Contributors to the Open Cluster Management project

package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/driftprogramming/pgxpoolmock"
	"github.com/golang/mock/gomock"
	"github.com/stolostron/search-v2-api/pkg/rbac"
)

// Test the liveness probe.
func TestLivenessProbe(t *testing.T) {
	// Create a request to pass to our handler. We don't have any query parameters for now, so we'll
	// pass 'nil' as the third parameter.
	req, err := http.NewRequest("GET", "/liveness", nil)
	if err != nil {
		t.Fatal(err)
	}

	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(livenessProbe)

	// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
	// directly and pass in our Request and ResponseRecorder.
	handler.ServeHTTP(rr, req)

	// Check the status code is what we expect.
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	// Check the response body is what we expect.
	expected := "OK"
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}

// mockPinger implements the Pinger interface for testing
type mockPinger struct {
	pingErr   error
	pingDelay time.Duration
}

func (m *mockPinger) Ping(ctx context.Context) error {
	if m.pingDelay > 0 {
		time.Sleep(m.pingDelay)
	}
	return m.pingErr
}

// mockPoolGetter mocks the PoolGetter interface for testing
type mockPoolGetter struct {
	pinger Pinger
}

func (m *mockPoolGetter) GetConnPool(ctx context.Context) Pinger {
	return m.pinger
}

// mockCacheGetter mocks the CacheGetter interface for testing
type mockCacheGetter struct {
	cache *rbac.Cache
}

func (m *mockCacheGetter) GetCache() *rbac.Cache {
	return m.cache
}

func TestReadinessProbe_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock pinger that succeeds
	mockPing := &mockPinger{
		pingErr: nil,
	}

	// Create mock pool for cache (doesn't need to do anything, just needs to exist)
	mockCachePool := pgxpoolmock.NewMockPgxPool(ctrl)

	// Create mock cache with healthy state using the test helper
	mockCache := rbac.NewMockCacheForTesting(true, mockCachePool)

	mockPG := &mockPoolGetter{pinger: mockPing}
	mockCG := &mockCacheGetter{cache: mockCache}

	// Create request
	req := httptest.NewRequest("GET", "/readiness", nil)
	rr := httptest.NewRecorder()

	// Execute handler with mocked dependencies
	readinessProbeWithDeps(rr, req, mockPG, mockCG)

	// Verify response
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Expected status code %v, got %v. Body: %s",
			http.StatusOK, status, rr.Body.String())
	}

	if body := rr.Body.String(); body != "OK" {
		t.Errorf("Expected body 'OK', got '%s'", body)
	}
}

func TestReadinessProbe_DBDown(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock pinger that fails
	mockPing := &mockPinger{
		pingErr: errors.New("database connection failed"),
	}

	// Create mock pool for cache
	mockCachePool := pgxpoolmock.NewMockPgxPool(ctrl)

	// Create mock cache with healthy state
	mockCache := rbac.NewMockCacheForTesting(true, mockCachePool)

	mockPG := &mockPoolGetter{pinger: mockPing}
	mockCG := &mockCacheGetter{cache: mockCache}

	// Create request
	req := httptest.NewRequest("GET", "/readiness", nil)
	rr := httptest.NewRecorder()

	// Execute handler with mocked dependencies
	readinessProbeWithDeps(rr, req, mockPG, mockCG)

	// Verify response
	if status := rr.Code; status != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %v, got %v. Body: %s",
			http.StatusServiceUnavailable, status, rr.Body.String())
	}

	body := rr.Body.String()
	if !strings.Contains(body, "database connection failed") {
		t.Errorf("Expected error message to contain 'database connection failed', got '%s'", body)
	}
}

func TestReadinessProbe_CacheUnhealthy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock pinger that succeeds
	mockPing := &mockPinger{
		pingErr: nil,
	}

	// Create mock cache with unhealthy state (pass nil pool to make it unhealthy)
	mockCache := rbac.NewMockCacheForTesting(false, nil)

	mockPG := &mockPoolGetter{pinger: mockPing}
	mockCG := &mockCacheGetter{cache: mockCache}

	// Create request
	req := httptest.NewRequest("GET", "/readiness", nil)
	rr := httptest.NewRecorder()

	// Execute handler with mocked dependencies
	readinessProbeWithDeps(rr, req, mockPG, mockCG)

	// Verify response
	if status := rr.Code; status != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %v, got %v. Body: %s",
			http.StatusServiceUnavailable, status, rr.Body.String())
	}

	body := rr.Body.String()
	if !strings.Contains(body, "RBAC cache unavailable") {
		t.Errorf("Expected error message to contain 'cache', got '%s'", body)
	}
}

func TestReadinessProbe_Timeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock pinger with slow response
	mockPing := &mockPinger{
		pingErr:   nil,
		pingDelay: 5 * time.Second, // Longer than the 2s timeout in readinessProbeWithDeps
	}

	// Create mock pool for cache
	mockCachePool := pgxpoolmock.NewMockPgxPool(ctrl)

	// Create mock cache with healthy state
	mockCache := rbac.NewMockCacheForTesting(true, mockCachePool)

	mockPG := &mockPoolGetter{pinger: mockPing}
	mockCG := &mockCacheGetter{cache: mockCache}

	// Create request with context (readinessProbeWithDeps creates its own 2s timeout)
	req := httptest.NewRequest("GET", "/readiness", nil)
	rr := httptest.NewRecorder()

	// Execute handler
	readinessProbeWithDeps(rr, req, mockPG, mockCG)

	// Verify response - should timeout
	if status := rr.Code; status != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %v, got %v. Body: %s",
			http.StatusServiceUnavailable, status, rr.Body.String())
	}

	body := rr.Body.String()
	if !strings.Contains(body, "timed out") {
		t.Errorf("Expected error message to contain 'timed out', got '%s'", body)
	}
}

func TestReadinessProbe_DBAndCacheDown(t *testing.T) {
	// Create mock pinger that fails
	mockPing := &mockPinger{
		pingErr: errors.New("database connection failed"),
	}

	// Create mock cache with unhealthy state
	mockCache := rbac.NewMockCacheForTesting(false, nil)

	mockPG := &mockPoolGetter{pinger: mockPing}
	mockCG := &mockCacheGetter{cache: mockCache}

	// Create request
	req := httptest.NewRequest("GET", "/readiness", nil)
	rr := httptest.NewRecorder()

	// Execute handler with mocked dependencies
	readinessProbeWithDeps(rr, req, mockPG, mockCG)

	// Verify response
	if status := rr.Code; status != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %v, got %v. Body: %s",
			http.StatusServiceUnavailable, status, rr.Body.String())
	}

	body := rr.Body.String()
	// Should contain errors from both DB and cache
	hasDBError := strings.Contains(body, "database connection failed")
	hasCacheError := strings.Contains(body, "cache")

	if !hasDBError || !hasCacheError {
		t.Errorf("Expected error message to contain both database and cache errors, got '%s'", body)
	}

	// Check for semicolon separator indicating combined errors between of async checks, error could be like:
	// 1. "database connection failed; RBAC cache unavailable" 2. "RBAC cache unavailable; database connection failed"
	if !strings.Contains(body, ";") {
		t.Errorf("Expected combined error with semicolon separator, got '%s'", body)
	}
}
