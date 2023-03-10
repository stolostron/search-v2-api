package metric

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gotest.tools/assert"
)

func TestDurationCode(t *testing.T) {

	registry := prometheus.NewRegistry()

	// mock http handler:
	httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	//mock HistogramVec
	durationHistogram := promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "http_request_duration_seconds_test",
			Help: "Duration of my request in seconds.",
		},
		[]string{"code", "query_type"},
	)
	registry.MustRegister(durationHistogram)

	//add mock label
	duration, _ := durationHistogram.CurryWith(prometheus.Labels{"query_type": "mock_query"})

	// mock instrumentHandlerDuration with mock observervec
	instrumentedHandler := promhttp.InstrumentHandlerDuration(
		duration,
		httpHandler,
	)

	//create a mock resquest to pass to handler:
	r, err := http.NewRequest("GET", "https://localhost:4010/searchapi/graphql", nil)
	if err != nil {
		t.Fatal(err)

	}
	// create response recorder so we can get status code and serve mock request
	resp := httptest.NewRecorder()
	instrumentedHandler.ServeHTTP(resp, r)

	// use the prometheus registry to confirm metrics have been scraped:
	metricFamilies, _ := registry.Gather()

	for _, mf := range metricFamilies {

		//assert our metric got collected:
		assert.Equal(t, mf.GetName(), "http_request_duration_seconds_test")

		//assert only one metric: probably redundant
		// assert.Equal(t, 1, len(mf.GetMetric()))

		for _, v := range mf.Metric[0].GetLabel() {

			//assert label code is 200
			if *v.Name == "code" {
				val := *v.Value
				assert.Equal(t, strconv.Itoa(resp.Code), val)
			}
			//assert label query_type is mock_query
			if *v.Name == "query_type" {
				val := *v.Value
				assert.Equal(t, "mock_query", val)
			}

		}

		for _, m := range mf.GetMetric() {
			//assert count of metric is one (one request)
			assert.Equal(t, uint64(1), m.GetHistogram().GetSampleCount())
		}

	}

}
