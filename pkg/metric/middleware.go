package metric

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func NewMetricsResponseWriter(w http.ResponseWriter) *metricsResponseWriter {
	return &metricsResponseWriter{w, 0}
}

func (lrw *metricsResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

// prometheusMiddleware implements mux.MiddlewareFunc.
func PrometheusMiddleware(next http.Handler) http.Handler {

	return promhttp.InstrumentHandlerDuration(HttpDuration,
		promhttp.InstrumentHandlerCounter(HttpRequestTotal, next))
}
