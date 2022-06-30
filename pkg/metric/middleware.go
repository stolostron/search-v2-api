package metric

import (
	"net/http"

	klog "k8s.io/klog/v2"

	"github.com/gorilla/mux"
)

// prometheusMiddleware implements mux.MiddlewareFunc.
func PrometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := mux.CurrentRoute(r)
		path, _ := route.GetPathTemplate()
		klog.V(4).Info("URL Referrer: %s", r.Referer())
		klog.V(4).Info("User Agent: %s", r.UserAgent())
		//Not working as intended. This does not capture the time taken by the entire call.
		//it only captures the time taken inside this call.
		/* timer := prometheus.NewTimer(HttpDuration.WithLabelValues(path, r.Method))
		timer.ObserveDuration()*/
		HttpRequestTotal.WithLabelValues(path, r.Method).Inc()
		next.ServeHTTP(w, r)

	})
}
