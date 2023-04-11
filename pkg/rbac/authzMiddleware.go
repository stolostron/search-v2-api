package rbac

import (
	"net/http"

	"k8s.io/klog/v2"
)

func AuthorizeUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// Trigger initialization of the shared cache. We should move this to a
		// different place where it's independent of the request.
		GetCache().shared.PopulateSharedCache(r.Context())

		_, userErr := GetCache().GetUserDataCache(r.Context(), nil)
		if userErr != nil {
			klog.Warning("Unexpected error while obtaining user data.", userErr)
		}

		klog.V(6).Info("User authorization successful!")
		klog.Info("** Request from: ", r.URL.Path, " URL details: ", r.URL)
		next.ServeHTTP(w, r.WithContext(r.Context()))

	})
}
