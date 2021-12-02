package main

import (
	"flag"
	"os"

	"github.com/open-cluster-management/search-api/pkg/config"
	"github.com/open-cluster-management/search-api/pkg/database"
	"github.com/open-cluster-management/search-api/pkg/server"

	klog "k8s.io/klog/v2"
)

func main() {
	// Initialize the logger.
	klog.InitFlags(nil)
	flag.Parse()
	defer klog.Flush()
	klog.Info("Starting search-api.")

	// Read the config from the environment.
	config := config.New()
	config.PrintConfig()
	database.GetConnection()
	if len(os.Args) > 1 && os.Args[1] == "playground" {
		server.StartAndListen(true)
	}
	server.StartAndListen(false)
}
