package rbac

import (
	"net/http"

	"k8s.io/klog/v2"
)

func CheckDBAvailability(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := GetCache()
		// if postgres db is not setup, return error
		if !c.dbConnInitialized {
			klog.Warning("Unable to handle request because we couldn't establish connection with database.")
			http.Error(w, "Unable to establish connection with database.", http.StatusServiceUnavailable)
			return
		}

		next.ServeHTTP(w, r.WithContext(r.Context()))

	})
}
