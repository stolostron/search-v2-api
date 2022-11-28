package main

import (
	"context"
	"flag"

	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/database"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	"github.com/stolostron/search-v2-api/pkg/server"
	klog "k8s.io/klog/v2"
)

func main() {
	// Initialize the logger.
	klog.InitFlags(nil)
	flag.Parse()
	defer klog.Flush()
	klog.Info("Starting search-v2-api.")

	// Read the config from the environment.
	config.Cfg.PrintConfig()

	// Validate required configuration to proceed.
	configError := config.Cfg.Validate()
	if configError != nil {
		klog.Fatal(configError)
	}

	// Get database connection
	database.GetConnection()

	// Watch the cache
	ctx := context.Background()
	go rbac.StartCacheValidation(ctx)

	server.StartAndListen()
}
