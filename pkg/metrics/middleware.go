package metrics

import (
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"
)

// Instrument with prom middleware to capture request metrics.
func PrometheusMiddleware(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		klog.V(5).Infof("Received request. User-Agent: %s RemoteAddr: %s", r.UserAgent(), r.RemoteAddr)

		// Skip Prometheus instrumentation for WebSocket upgrades
		if r.Header.Get("Upgrade") == "websocket" {
			klog.V(5).Info("Skipping Prometheus instrumentation for WebSocket upgrade")
			next.ServeHTTP(w, r)
			return
		}

		curriedRequestDuration, err := RequestDuration.CurryWith(prometheus.Labels{
			"remoteAddr": r.RemoteAddr[0:strings.LastIndex(r.RemoteAddr, ":")], // Remove port
			"userAgent":  r.UserAgent(),
		})
		if err != nil {
			klog.Error("Error while curring the RequestDuration metric with remoteAddr label. ", err)
		}
		h := promhttp.InstrumentHandlerDuration(curriedRequestDuration, next)
		h.ServeHTTP(w, r)
	})
}
