package server

import (
	"context"
	"github.com/gorilla/websocket"
	"github.com/stolostron/search-v2-api/pkg/config"
	"net/http"
	"time"

	klog "k8s.io/klog/v2"
)

// RequestTimeout adds a timeout to the request context to prevent unbounded connection growth
func RequestTimeout(cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			timeout := time.Duration(cfg.RequestTimeout) * time.Millisecond
			requestType := "request"
			if websocket.IsWebSocketUpgrade(r) {
				timeout = time.Duration(cfg.StreamRequestTimeout) * time.Millisecond
				requestType = "stream"
			}
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			klog.V(5).Infof("Timeout of %v applied to %s context", timeout, requestType)
			defer cancel()

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
