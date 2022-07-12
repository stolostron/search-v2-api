package rbac

import (
	"net/http"

	"github.com/stolostron/search-v2-api/pkg/metric"
	"k8s.io/klog/v2"
)

func AuthorizeUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		_, err := cacheInst.ClusterScopedResources(r.Context())
		if err != nil {
			klog.Warning("Unexpected error while obtaining cluster-scoped resources.", err)
			metric.AuthzFailed.WithLabelValues("UnexpectedAuthzError").Inc()
		}
		klog.Info("Finished getting shared resources. Now gettng user data..")

		clientToken := r.Context().Value(ContextAuthTokenKey).(string)
		_, newerr := cacheInst.GetUserData(r.Context(), clientToken)
		if newerr != nil {
			klog.Warning("Unexpected error while obtaining user namesapces.", newerr)
		}

		klog.V(5).Info("User authorization successful!")
		next.ServeHTTP(w, r.WithContext(r.Context()))

	})
}
