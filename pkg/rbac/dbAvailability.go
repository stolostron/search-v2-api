package rbac

import (
	"net/http"

	"k8s.io/klog/v2"
)

func CheckDBAvailability(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// if postgres db is not setup, return error
		if !GetCache().dbConnInitialized {
			http.Error(w, "Unable to establish connection with database.", http.StatusServiceUnavailable)
			return
		}

		klog.V(6).Info("Connection with database successful!")
		next.ServeHTTP(w, r.WithContext(r.Context()))

	})
}
