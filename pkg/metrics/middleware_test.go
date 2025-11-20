package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_PrometheusInstrumentation(t *testing.T) {
	// Create a mock resquest to pass to handler.
	req := httptest.NewRequest("POST", "https://localhost:4010/searchapi/graphql", nil)
	res := httptest.NewRecorder()

	// Execute middleware function.
	httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	promMiddle := PrometheusMiddleware(httpHandler)
	promMiddle.ServeHTTP(res, req)

	// Validate the collected metrics.

	collectedMetrics, _ := PromRegistry.Gather() // use the prometheus registry to confirm metrics have been scraped.
	assert.Equal(t, 5, len(collectedMetrics))    // Validate total metrics collected.

	// METRIC 1: search_api_db_connection_failed
	assert.Equal(t, "search_api_db_connection_failed", collectedMetrics[0].GetName())
	assert.Equal(t, float64(0), collectedMetrics[1].Metric[0].GetCounter().GetValue())

	// METRIC 2:  search_api_request_duration
	assert.Equal(t, "search_api_request_duration", collectedMetrics[1].GetName())
	assert.Equal(t, 3, len(collectedMetrics[1].Metric[0].GetLabel()))
	assert.Equal(t, "code", *collectedMetrics[1].Metric[0].GetLabel()[0].Name)
	assert.Equal(t, "200", *collectedMetrics[1].Metric[0].GetLabel()[0].Value)
	assert.Equal(t, uint64(1), collectedMetrics[1].Metric[0].GetHistogram().GetSampleCount())

	// METRIC 3: search_api_db_query_duration
	// Not generated in this scenario because there's no queries triggered by this test.

	// METRIC 4: search_api_subscriptions_active
	assert.Equal(t, "search_api_subscriptions_active", collectedMetrics[3].GetName())

	// METRIC 5: search_api_websocket_connections_total
	assert.Equal(t, "search_api_websocket_connections_total", collectedMetrics[4].GetName())
}

func Test_PrometheusMiddleware_WebSocketUpgradeSkip(t *testing.T) {
	// Track if the handler was called
	handlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Create a WebSocket upgrade request
	req := httptest.NewRequest("GET", "https://localhost:4010/searchapi/graphql", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	res := httptest.NewRecorder()

	// Get metrics count before the request
	collectedMetricsBefore, _ := PromRegistry.Gather()
	requestMetricsBefore := collectedMetricsBefore[1].Metric

	// Execute middleware function
	promMiddle := PrometheusMiddleware(nextHandler)
	promMiddle.ServeHTTP(res, req)

	// Validate the handler was called
	assert.True(t, handlerCalled, "Handler should be called for WebSocket upgrade")
	assert.Equal(t, http.StatusOK, res.Code)

	// Get metrics count after the request
	collectedMetricsAfter, _ := PromRegistry.Gather()
	requestMetricsAfter := collectedMetricsAfter[1].Metric

	// The request duration metric should NOT have increased for WebSocket upgrades
	// (they skip Prometheus instrumentation)
	assert.Equal(t, len(requestMetricsBefore), len(requestMetricsAfter),
		"WebSocket upgrades should not add request duration metrics")
}

func Test_PrometheusMiddleware_NonWebSocketInstrumented(t *testing.T) {
	// Track if the handler was called
	handlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Create a regular POST request (not WebSocket) with unique remote address
	req := httptest.NewRequest("POST", "https://localhost:4010/searchapi/graphql", nil)
	req.RemoteAddr = "192.168.1.100:12345" // Unique remote address for this test
	res := httptest.NewRecorder()

	// Execute middleware function
	promMiddle := PrometheusMiddleware(nextHandler)
	promMiddle.ServeHTTP(res, req)

	// Validate the handler was called
	assert.True(t, handlerCalled, "Handler should be called for normal request")
	assert.Equal(t, http.StatusOK, res.Code)

	// Get metrics after the request
	collectedMetricsAfter, _ := PromRegistry.Gather()

	// Validate that request duration metrics were collected
	assert.Equal(t, "search_api_request_duration", collectedMetricsAfter[1].GetName())
	assert.Greater(t, len(collectedMetricsAfter[1].Metric), 0,
		"Normal requests should have request duration metrics")
}

func Test_PrometheusMiddleware_WebSocketWrongUpgradeValue(t *testing.T) {
	// Track if the handler was called
	handlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Create a request with Upgrade header but wrong value
	req := httptest.NewRequest("GET", "https://localhost:4010/searchapi/graphql", nil)
	req.Header.Set("Upgrade", "http2")     // Not a WebSocket upgrade
	req.RemoteAddr = "192.168.1.101:12346" // Unique remote address for this test
	res := httptest.NewRecorder()

	// Execute middleware function
	promMiddle := PrometheusMiddleware(nextHandler)
	promMiddle.ServeHTTP(res, req)

	// Validate the handler was called
	assert.True(t, handlerCalled, "Handler should be called")
	assert.Equal(t, http.StatusOK, res.Code)

	// Get metrics after the request
	collectedMetricsAfter, _ := PromRegistry.Gather()

	// Validate that request duration metrics were collected
	assert.Equal(t, "search_api_request_duration", collectedMetricsAfter[1].GetName())
	assert.Greater(t, len(collectedMetricsAfter[1].Metric), 0,
		"Non-WebSocket requests should have request duration metrics")
}
