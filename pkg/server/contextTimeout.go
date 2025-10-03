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
			var ctx context.Context
			var cancel context.CancelFunc
			if websocket.IsWebSocketUpgrade(r) {
				ctx, cancel = context.WithTimeout(r.Context(), time.Duration(cfg.StreamRequestTimeout)*time.Millisecond)
				klog.V(5).Infof("Timeout of %v applied to stream request context", time.Duration(cfg.StreamRequestTimeout)*time.Millisecond)
			} else {
				ctx, cancel = context.WithTimeout(r.Context(), time.Duration(cfg.RequestTimeout)*time.Millisecond)
				klog.V(5).Infof("Timeout of %v applied to request context", time.Duration(cfg.RequestTimeout)*time.Millisecond)
			}
			defer cancel()

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
