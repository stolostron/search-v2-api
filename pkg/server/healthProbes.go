// Copyright Contributors to the Open Cluster Management project

package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/stolostron/search-v2-api/pkg/database"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	"k8s.io/klog/v2"
)

// LivenessProbe is used to check if this service is alive.
func livenessProbe(w http.ResponseWriter, r *http.Request) {
	klog.V(5).Info("livenessProbe")
	fmt.Fprint(w, "OK")
}

// ReadinessProbe checks if rbac cache and database are available.
func readinessProbe(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	dbDone := make(chan error, 1)
	go func() {

		// pass background context to avoid timing out of this request initializes pool
		pool := database.GetConnPool(context.Background())
		if pool == nil {
			dbDone <- fmt.Errorf("database pool not initialized")
			return
		}
		dbDone <- pool.Ping(context.Background())
	}()

	cacheDone := make(chan error, 1)
	go func() {
		cache := rbac.GetCache()
		if cache == nil {
			cacheDone <- fmt.Errorf("RBAC cache not initialized")
			return
		}
		if !cache.IsHealthy() {
			cacheDone <- fmt.Errorf("RBAC cache unavailable")
			return
		}
		cacheDone <- nil
	}()

	select {
	case err := <-dbDone:
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
	case err := <-cacheDone:
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
	case <-ctx.Done():
		http.Error(w, "Readiness probe timed out", http.StatusServiceUnavailable)
		return
	}

	fmt.Fprint(w, "OK")
}
