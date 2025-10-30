// Copyright Contributors to the Open Cluster Management project

//go:build development
// +build development

// This file is excluded from compilation unless the build flag -tags development is used.
// Use `make run` to run with the development flag.
package config

import (
	"os"

	"k8s.io/klog/v2"
)

func init() {
	klog.Warning("!!! Running in development mode. !!!")
	DEVELOPMENT_MODE = true
	os.Setenv("FEATURE_FEDERATED_SEARCH", "true")
	os.Setenv("FEATURE_FINE_GRAINED_RBAC", "true")
	os.Setenv("FEATURE_SUBSCRIPTION", "true")
	Cfg = new()
}
