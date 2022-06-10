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
		klog.Info("Finished getting cluster-scoped resources. Now Authorizing..")

	})
}
