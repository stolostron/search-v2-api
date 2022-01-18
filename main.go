package main

import (
	"flag"
	"os"

	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/database"
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
	config := config.New()
	config.PrintConfig()
	database.GetConnection()
	if len(os.Args) > 1 && os.Args[1] == "playground" {
		server.StartAndListen(true)
	}
	server.StartAndListen(false)
}
