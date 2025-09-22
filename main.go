package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/database"
	"github.com/stolostron/search-v2-api/pkg/notification"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Establish the database connection.
	database.GetConnPool(ctx)

	// Set up PostgreSQL triggers for notifications if enabled
	if config.Cfg.Features.NotificationEnabled {
		if err := notification.SetupDatabaseTriggers(ctx); err != nil {
			klog.Errorf("Failed to set up database triggers: %v", err)
			klog.Warning("Notifications will be disabled")
		} else {
			// Initialize and start the notification listener
			manager := notification.GetNotificationManager()
			if err := manager.Start(ctx); err != nil {
				klog.Errorf("Failed to start notification manager: %v", err)
				klog.Warning("Notifications will be disabled")
			} else {
				// Set up graceful shutdown for notifications
				go func() {
					sigChan := make(chan os.Signal, 1)
					signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
					<-sigChan
					klog.Info("Shutting down notification manager...")
					manager.Stop()
					cancel()
					<-sigChan
					klog.Info("Second signal received. Exiting...")
					os.Exit(1) // Second signal. Exit directly.
				}()
			}
		}
	}

	// Start process to watch the RBAC config andd update the cache.
	go rbac.GetCache().StartBackgroundValidation(ctx)

	server.StartAndListen()
}
