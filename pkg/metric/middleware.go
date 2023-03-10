package metric

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Instrument with prom middleware to capture request metrics.
func PrometheusMiddleware(next http.Handler) http.Handler {

	// InstrumentHandlerDuration is a middleware that wraps the provided http.Handler to observe the
	// request duration with the provided ObserverVec.
	return promhttp.InstrumentHandlerDuration(HttpDuration, next)
}
