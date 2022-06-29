package metric

import (
	"net/http"

	klog "k8s.io/klog/v2"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
)

// prometheusMiddleware implements mux.MiddlewareFunc.
func PrometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := mux.CurrentRoute(r)
		path, _ := route.GetPathTemplate()
		timer := prometheus.NewTimer(HttpDuration.WithLabelValues(path, r.Method))
		klog.Infof("xxxxxxxxxxxxxxxx URL Referrer: %s", r.Referer())
		klog.Infof("xxxxxxxxxxxxxxxx User Agent: %s", r.UserAgent())
		next.ServeHTTP(w, r)
		timer.ObserveDuration()
		HttpRequestTotal.WithLabelValues(path, r.Method).Inc()

		/* now := time.Now()
		d := promhttp.newDelegator(w, nil)
		next.ServeHTTP(d, r)

		HttpDuration.WithLabelValues(path, d.Status(), r.Method).Observe(time.Since(now).Seconds()) */
	})
}
