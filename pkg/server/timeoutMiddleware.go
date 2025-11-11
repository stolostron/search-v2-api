package server

import (
	"net/http"
	"time"

	klog "k8s.io/klog/v2"
)

// TimeoutHandler wraps http next handler with timeout handler to prevent unbound request/connection growth
func TimeoutHandler(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// NOTE: This problem started unexpectedly after working for a while.
			//       This fixes the problem, but it's not clear why it started happening.

			// Skip timeout handling for WebSocket upgrades
			// The http.TimeoutHandler wraps the ResponseWriter in a way that doesn't implement http.Hijacker,
			// which is required for WebSocket upgrades
			if r.Header.Get("Upgrade") == "websocket" {
				klog.V(8).Info("Skipping timeout handler for WebSocket upgrade")
				next.ServeHTTP(w, r)
				return
			}

			klog.V(8).Infof("Applying %v timeout for HTTP request: %s %s", timeout, r.Method, r.URL.Path)
			handler := http.TimeoutHandler(next, timeout, "Request timed out")
			handler.ServeHTTP(w, r)
		})
	}
}
