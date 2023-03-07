package metric

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// prometheusMiddleware implements mux.MiddlewareFunc.
func PrometheusMiddleware(next http.Handler) http.Handler {

	return promhttp.InstrumentHandlerDuration(HttpDuration,
		promhttp.InstrumentHandlerCounter(HttpRequestTotal, next))
}
