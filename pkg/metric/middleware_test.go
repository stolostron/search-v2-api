package metric

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	// "github.com/prometheus/client_golang/prometheus/promauto"
	"gotest.tools/assert"
)

func Test_DurationCode(t *testing.T) {
	registry := prometheus.NewRegistry()

	// mock http handler
	httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	// create a mock resquest to pass to handler
	r := httptest.NewRequest("POST", "https://localhost:4010/searchapi/graphql", nil)
	// create response recorder so we can get status code and serve mock request
	resp := httptest.NewRecorder()

	// Execute middleware function.
	promMiddle := PrometheusMiddleware(httpHandler)
	promMiddle.ServeHTTP(resp, r)

	// Validate result.
	// use the prometheus registry to confirm metrics have been scraped:
	metricFamilies, _ := registry.Gather()
	for _, mf := range metricFamilies {
		//assert our metric got collected:
		assert.Equal(t, mf.GetName(), "http_request_duration_seconds_test")

		for _, v := range mf.Metric[0].GetLabel() {
			// assert label code is 200
			if *v.Name == "code" {
				val := *v.Value
				assert.Equal(t, strconv.Itoa(resp.Code), val)
			}
		}

		for _, m := range mf.GetMetric() {
			// assert count of metric is one (one request)
			assert.Equal(t, uint64(1), m.GetHistogram().GetSampleCount())
		}
	}

}
