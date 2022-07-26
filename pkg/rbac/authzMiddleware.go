package rbac

import (
	"net/http"

	"github.com/stolostron/search-v2-api/pkg/metric"
	"k8s.io/klog/v2"
)

func AuthorizeUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		//Hub Cluster resources authorization:
		_, err := cacheInst.ClusterScopedResources(r.Context())
		if err != nil {
			klog.Warning("Unexpected error while obtaining cluster-scoped resources.", err)
			metric.AuthzFailed.WithLabelValues("UnexpectedAuthzError").Inc()
		}
		klog.Info("Finished getting shared resources. Now getting user data..")

		clientToken := r.Context().Value(ContextAuthTokenKey).(string)
		_, userErr := cacheInst.GetUserData(r.Context(), clientToken, nil)
		if userErr != nil {
			klog.Warning("Unexpected error while obtaining user namespaces.", userErr)
		}

		//Managed Cluster resources authorization:
		// userData.getManagedClusterResources()

		klog.V(5).Info("User authorization successful!")
		next.ServeHTTP(w, r.WithContext(r.Context()))

	})
}
