package server

import (
	"crypto/tls"
	"fmt"
	"net/http"

	klog "k8s.io/klog/v2"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gorilla/mux"
	"github.com/stolostron/search-v2-api/graph"
	"github.com/stolostron/search-v2-api/graph/generated"
	"github.com/stolostron/search-v2-api/pkg/config"
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
	srv := &http.Server{
		Addr:         config.Cfg.API_SERVER_URL,
		Handler:      handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: &graph.Resolver{}})),
		TLSConfig:    cfg,
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}

	// Initiate router
	router := mux.NewRouter()
	router.HandleFunc("/liveness", livenessProbe).Methods("GET")
	router.HandleFunc("/readiness", readinessProbe).Methods("GET")

	if config.Cfg.PlaygroundMode {
		router.Handle("/playground", playground.Handler("GraphQL playground", fmt.Sprintf("%s/graphql", config.Cfg.ContextPath)))
		klog.Infof("GraphQL playground is now running on https://localhost:%d/playground", port)
	}

	// Add authentication middleware to the /searchapi (ContextPath) subroute.
	apiSubrouter := router.PathPrefix(config.Cfg.ContextPath).Subrouter()
	apiSubrouter.Use(rbac.Middleware())
	apiSubrouter.Handle("/graphql", srv.Handler)

	klog.Infof(`Search API is now running on https://localhost:%d%s/graphql`, port, config.Cfg.ContextPath)
	klog.Fatal(http.ListenAndServeTLS(":"+fmt.Sprint(port), "./sslcert/tls.crt", "./sslcert/tls.key", router))
}
