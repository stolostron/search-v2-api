package rbac

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/klog/v2"
)

func AuthorizeUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		//get user cluster-scoped resources and cache:
		// clientToken := r.Context().Value(ContextAuthTokenKey).(string)
		// uid := cache.tokenReviews[clientToken].tokenReview.UID
		_, err := Instcache.pool.Query(context.Background(), `Select Count(*) from search.resources`, nil)
		fmt.Println("Instacache pool works", err)
		// _, err := Instcache.checkUserResources()
		if err != nil {
			klog.Warning("Unexpected error while obtaining cluster-scoped resources.", err)
		}
		fmt.Println("Finished getting cluster-scoped resources. Now Authorizing..") //place-holder comment.

	})
}
