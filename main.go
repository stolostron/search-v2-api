package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	ctx, exitRoutines := context.WithCancel(context.Background())

	// Establish the database connection.
	database.GetConnPool(ctx)

	// Start process to watch the RBAC config andd update the cache.
	go rbac.GetCache().StartBackgroundValidation(ctx)

	go server.StartAndListen(ctx)

	// Listen and wait for termination signal.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigs // Waits for termination signal.
	klog.Warningf("Received termination signal %s. Exiting server. ", sig)
	// Stop the Postgres listener.
	database.StopPostgresListener()
	exitRoutines()

	time.Sleep(5 * time.Second)
	klog.Warning("Exiting search-api.")
}
