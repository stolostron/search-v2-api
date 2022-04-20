package main

import (
	"flag"
	"os"

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

	//Call kubeclient for authentication
	rbac.KubeClient()

	//Get database connection
	database.GetConnection()

	if len(os.Args) > 1 && os.Args[1] == "playground" {
		server.StartAndListen(true)
	}
	server.StartAndListen(false)
}
