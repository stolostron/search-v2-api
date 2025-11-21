package rbac

import (
	"net/http"

	"k8s.io/klog/v2"
)

func AuthorizeUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// Skip authorization middleware for WebSocket connections
		if r.Header.Get("Upgrade") == "websocket" {
			klog.V(1).Info("Skipping authorization middleware for WebSocket connection.")
			next.ServeHTTP(w, r)
			return
		}

		// Trigger initialization of the shared cache. We should move this to a
		// different place where it's independent of the request.
		GetCache().shared.PopulateSharedCache(r.Context())

		_, userErr := GetCache().GetUserDataCache(r.Context(), nil)
		if userErr != nil {
			klog.Warning("Unexpected error while obtaining user data.", userErr)
		}

		klog.V(6).Info("User authorization successful!")
		next.ServeHTTP(w, r.WithContext(r.Context()))

	})
}
