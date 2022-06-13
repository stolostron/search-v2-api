package rbac

import (
	"net/http"

	"k8s.io/klog/v2"
)

func AuthorizeUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		_, err := cacheInst.ClusterScopedResources(r.Context())
		if err != nil {
			klog.Warning("Unexpected error while obtaining cluster-scoped resources.", err)
		}

		_, newerr := cacheInst.NamespacedResources(r.Context())
		if newerr != nil {
			klog.Warning()
		}

		klog.Info("Finished getting resources. Now Authorizing..")
		next.ServeHTTP(w, r.WithContext(r.Context()))

	})
}
