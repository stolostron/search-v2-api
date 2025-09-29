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
	klog.V(5).Info("readinessProbe")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	pool := database.GetConnPool(ctx)
	if pool == nil {
		http.Error(w, "Database pool not initialized", http.StatusServiceUnavailable)
		return
	}
	if err := pool.Ping(ctx); err != nil {
		http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
		return
	}

	// Check RBAC cache
	cache := rbac.GetCache()
	if cache == nil {
		http.Error(w, "RBAC cache not initialized", http.StatusServiceUnavailable)
		return
	}
	if !cache.IsHealthy() {
		http.Error(w, "RBAC cache unavailable", http.StatusServiceUnavailable)
		return
	}

	fmt.Fprint(w, "OK")
}
