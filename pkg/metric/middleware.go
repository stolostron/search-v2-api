package metric

import (
	"net/http"

	klog "k8s.io/klog/v2"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
)

type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func NewMetricsResponseWriter(w http.ResponseWriter) *metricsResponseWriter {
	return &metricsResponseWriter{w, http.StatusOK}
}

func (lrw *metricsResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

// prometheusMiddleware implements mux.MiddlewareFunc.
func PrometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		klog.Info("At PrometheusMiddleware BEFORE processing the request.")
		route := mux.CurrentRoute(r)
		path, _ := route.GetPathTemplate()

		timer := prometheus.NewTimer(HttpDuration.WithLabelValues(path, r.Method))
		HttpRequestTotal.WithLabelValues(path, r.Method).Inc()

		metricsRespWriter := NewMetricsResponseWriter(w)

		// This will run at the end of the middleware chain.
		defer func() {
			klog.Infof("At prom middleware AFTER processing the request. response_code  %+v", metricsRespWriter.statusCode)
			timer.ObserveDuration()
		}()

		next.ServeHTTP(metricsRespWriter, r)
	})
}
