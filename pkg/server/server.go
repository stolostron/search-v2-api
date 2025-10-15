package server

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	klog "k8s.io/klog/v2"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stolostron/search-v2-api/graph"
	"github.com/stolostron/search-v2-api/graph/generated"
	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/federated"
	"github.com/stolostron/search-v2-api/pkg/metrics"
	"github.com/stolostron/search-v2-api/pkg/rbac"
)

func StartAndListen() {
	port := config.Cfg.HttpPort

	// Configure TLS
	cfg := &tls.Config{
		MinVersion:               tls.VersionTLS12,
		CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		},
	}

	// Initiate router
	router := mux.NewRouter()
	router.HandleFunc("/liveness", livenessProbe).Methods("GET")
	router.HandleFunc("/readiness", readinessProbe).Methods("GET")
	router.Handle("/metrics", promhttp.HandlerFor(metrics.PromRegistry, promhttp.HandlerOpts{})).Methods("GET")

	if config.Cfg.PlaygroundMode {
		router.Handle("/playground",
			playground.Handler("Search GraphQL playground", fmt.Sprintf("%s/graphql", config.Cfg.ContextPath)))
		klog.Infof("GraphQL playground is now running on https://localhost:%d/playground", port)
	}

	if config.Cfg.Features.FederatedSearch {
		klog.Infof("Federated search is enabled.")
		fedSubrouter := router.PathPrefix("/federated").Subrouter()
		fedSubrouter.Use(rbac.AuthenticateUser)
		// fedSubrouter.Use(metrics.PrometheusMiddleware)  // FUTURE: Add prometheus metric for federated requests.
		// fedSubrouter.Use(federated.GetConfig)           // TODO: Add a health check for federated services.
		fedSubrouter.HandleFunc("", federated.HandleFederatedRequest).Methods("POST")
	} else {
		klog.Infof("Federated search is disabled. To enable set env variable FEATURES_FEDERATED_SEARCH=true")
	}

	// Add authentication middleware to the /searchapi (ContextPath) subroute.
	apiSubrouter := router.PathPrefix(config.Cfg.ContextPath).Subrouter()

	apiSubrouter.Use(TimeoutHandler(time.Duration(config.Cfg.RequestTimeout)))
	apiSubrouter.Use(metrics.PrometheusMiddleware)
	apiSubrouter.Use(rbac.CheckDBAvailability)
	apiSubrouter.Use(rbac.AuthenticateUser)
	apiSubrouter.Use(rbac.AuthorizeUser)

	defaultSrv := handler.NewDefaultServer(generated.NewExecutableSchema(
		generated.Config{Resolvers: &graph.Resolver{}}))
	defaultSrv.AddTransport(&transport.Websocket{})
	apiSubrouter.Handle("/graphql", defaultSrv)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           router,
		TLSConfig:         cfg,
		TLSNextProto:      make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	klog.Infof(`Search API is now running on https://localhost:%d%s/graphql`, port, config.Cfg.ContextPath)
	serverErr := srv.ListenAndServeTLS("./sslcert/tls.crt", "./sslcert/tls.key")
	if serverErr != nil {
		klog.Fatal("Server process ended with error. ", serverErr)
	}
}
