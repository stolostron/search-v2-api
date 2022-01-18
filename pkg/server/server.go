package server

import (
	"crypto/tls"
	"fmt"
	"net/http"

	klog "k8s.io/klog/v2"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/stolostron/search-v2-api/graph"
	"github.com/stolostron/search-v2-api/graph/generated"
	"github.com/stolostron/search-v2-api/pkg/config"
)

// const defaultPort = "8080"

func StartAndListen(playmode bool) {
	port := config.Cfg.HttpPort

	// router := mux.NewRouter()ß

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

	http.Handle("/", playground.Handler("GraphQL playground", "/query"))
	http.Handle("/query", srv.Handler)
	if playmode {
		klog.Infof("connect to https://localhost:%d%s/graphql for GraphQL playground", port, config.Cfg.ContextPath)
		klog.Fatal(http.ListenAndServeTLS(":"+fmt.Sprint(port), "./sslcert/tls.crt", "./sslcert/tls.key",
			nil))
	}
	klog.Infof(`Search API is now running on https://localhost:%d%s/graphql`, port, config.Cfg.ContextPath)
	klog.Fatal(http.ListenAndServeTLS(":"+fmt.Sprint(port), "./sslcert/tls.crt", "./sslcert/tls.key",
		srv.Handler))
}
