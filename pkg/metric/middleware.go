package metric

import (
	"net/http"

	klog "k8s.io/klog/v2"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
)

// prometheusMiddleware implements mux.MiddlewareFunc.
func ExposeMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := mux.CurrentRoute(r)
		path, _ := route.GetPathTemplate()
		klog.V(4).Infof("URL Referrer: %s", r.Referer())
		klog.V(4).Infof("User Agent: %s", r.UserAgent())

		timer := prometheus.NewTimer(HttpDuration.WithLabelValues(path, r.Method, "serve_http_request"))
		defer timer.ObserveDuration()

		HttpRequestTotal.WithLabelValues(path, r.Method).Inc()

		next.ServeHTTP(w, r)

	})
}
