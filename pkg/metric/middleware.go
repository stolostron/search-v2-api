package metric

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Instrument with prom middleware to capture request metrics.
func PrometheusMiddleware(next http.Handler) http.Handler {

	queryType := "searchWithRelationship" // TODO: Need to extract this from the request.
	duration, _ := HttpDuration.CurryWith(prometheus.Labels{"query_type": queryType})
	// InstrumentHandlerDuration is a middleware that wraps the provided http.Handler to observe the
	// request duration with the provided ObserverVec.
	return promhttp.InstrumentHandlerDuration(duration,
		promhttp.InstrumentHandlerCounter(HttpRequestTotal, next))
}
