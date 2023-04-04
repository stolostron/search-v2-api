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
	assert.Equal(t, 1, len(collectedMetrics))    // Validate total metrics collected.

	// METRIC 1:  search_api_request_duration
	assert.Equal(t, "search_api_request_duration", collectedMetrics[0].GetName())
	assert.Equal(t, 1, len(collectedMetrics[0].Metric[0].GetLabel()))
	assert.Equal(t, "code", *collectedMetrics[0].Metric[0].GetLabel()[0].Name)
	assert.Equal(t, "200", *collectedMetrics[0].Metric[0].GetLabel()[0].Value)
	assert.Equal(t, uint64(1), collectedMetrics[0].Metric[0].GetHistogram().GetSampleCount())

	// METRIC 2: search_api_db_connection_failed
	// Not generated in this scenario because there's no database connection for unit tests.

	// METRIC 3: search_api_db_query_duration
	// Not generated in this scenario because there's no database connection for unit tests.
}
