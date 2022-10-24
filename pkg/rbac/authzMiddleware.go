package rbac

import (
	"net/http"
	"time"

	db "github.com/stolostron/search-v2-api/pkg/database"
	"github.com/stolostron/search-v2-api/pkg/metric"
	"k8s.io/klog/v2"
)

func AuthorizeUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		//for earch request we want to check if we can get database connection:
		GetCache().dbconnLock.Lock()
		defer GetCache().dbconnLock.Unlock()

		//verify last time checked was no longer than 2 minutes:
		if GetCache().lastCheckTime.After(time.Now().Add(time.Duration(-2) * time.Minute)) {
			conn := db.GetConnection()
			if conn != nil {
				GetCache().pool = conn
				GetCache().lastCheckTime = time.Now()
			}
		}

		// Hub Cluster resources authorization:
		err := GetCache().PopulateSharedCache(r.Context())
		if err != nil {
			klog.Warning("Unexpected error while obtaining shared resources.", err)
			metric.AuthzFailed.WithLabelValues("UnexpectedAuthzError").Inc()
		}
		klog.V(6).Info("Finished getting shared resources. Now getting user data.")

		_, userErr := GetCache().GetUserDataCache(r.Context(), nil)
		if userErr != nil {
			klog.Warning("Unexpected error while obtaining user data.", userErr)
		}

		klog.V(6).Info("User authorization successful!")
		next.ServeHTTP(w, r.WithContext(r.Context()))

	})
}
