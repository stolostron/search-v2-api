package main

import (
	"flag"

	// "net/http"
	"os"

	"github.com/go-chi/chi"
	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/database"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	"github.com/stolostron/search-v2-api/pkg/server"

	// "k8s.io/client-go/informers/rbac"
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

	database.GetConnection()

	router := chi.NewRouter()
	router.Use(rbac.Middleware())

	if len(os.Args) > 1 && os.Args[1] == "playground" {

		// router.Handle("/", playground.Handler("searchapi/graphql", "/playground"))
		// log.Fatal(http.ListenAndServe("localhost:4010", router))

		server.StartAndListen(true, router)
	}

	server.StartAndListen(false, router)

}
