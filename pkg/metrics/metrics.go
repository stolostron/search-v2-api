// Copyright Contributors to the Open Cluster Management project
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	PromRegistry = prometheus.NewRegistry()

	RequestDuration = promauto.With(PromRegistry).NewHistogramVec(prometheus.HistogramOpts{
		Name: "search_api_request_duration",
		Help: "Time (seconds) the search api took to process the request",
	}, []string{"code"})

	DBConnectionFailed = promauto.With(PromRegistry).NewCounterVec(prometheus.CounterOpts{
		Name: "search_api_db_connection_failed",
		Help: "The number of failed database connection attempts.",
	}, []string{})

	DBQueryDuration = promauto.With(PromRegistry).NewHistogramVec(prometheus.HistogramOpts{
		Name: "search_api_db_query_duration",
		Help: "Latency (seconds) for database queries.",
	}, []string{"query_name"})
)
