package server

import (
	klog "k8s.io/klog/v2"
	"net/http"
	"time"
)

// TimeoutHandler wraps http next handler with timeout handler to prevent unbound request/connection growth
func TimeoutHandler(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			klog.V(5).Infof("Applying %v timeout for HTTP request: %s %s", timeout, r.Method, r.URL.Path)
			handler := http.TimeoutHandler(next, timeout, "Request timed out")
			handler.ServeHTTP(w, r)
		})
	}
}
