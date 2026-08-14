// Copyright Contributors to the Open Cluster Management project

package server

import (
	"fmt"
	"net/http"

	"k8s.io/klog/v2"
)

// LivenessProbe is used to check if this service is alive.
func livenessProbe(w http.ResponseWriter, r *http.Request) {
	klog.V(5).Info("livenessProbe")
	_, _ = fmt.Fprint(w, "OK")
}

// ReadinessProbe checks if database is available.
func readinessProbe(w http.ResponseWriter, r *http.Request) {
	klog.V(5).Info("readinessProbe")
	_, _ = fmt.Fprint(w, "OK")
}
