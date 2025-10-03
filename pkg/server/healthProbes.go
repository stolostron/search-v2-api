// Copyright Contributors to the Open Cluster Management project

package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"
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
	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		// pass background context to avoid timing out of this request initializes pool
		pool := database.GetConnPool(context.Background())
		if pool == nil {
			errCh <- fmt.Errorf("database pool not initialized")
			return
		}
		errCh <- pool.Ping(context.Background())
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		cache := rbac.GetCache()
		if cache == nil {
			errCh <- fmt.Errorf("RBAC cache not initialized")
			return
		}
		if !cache.IsHealthy() {
			errCh <- fmt.Errorf("RBAC cache unavailable")
			return
		}
		errCh <- nil
	}()

	// block for errs in goroutine to permit timeout to return in case either checks hang
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		close(errCh)
		combinedErr := ""
		// build one err string for all failed readiness sub checks ; separated
		for e := range errCh {
			if e != nil {
				if combinedErr == "" {
					combinedErr = e.Error()
				} else {
					combinedErr = fmt.Sprintf("%s; %v", combinedErr, e)
				}
			}
		}
		if combinedErr != "" {
			http.Error(w, combinedErr, http.StatusServiceUnavailable)
		}

		fmt.Fprint(w, "OK")
	case <-ctx.Done():
		http.Error(w, "Readiness probe timed out", http.StatusServiceUnavailable)
		return
	}
}
