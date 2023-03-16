// Copyright Contributors to the Open Cluster Management project
package metric

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	PromRegistry = prometheus.NewRegistry()

	HttpRequestsHistogram = promauto.With(PromRegistry).NewHistogramVec(prometheus.HistogramOpts{
		Name: "search_api_requests",
		Help: "Histogram of HTTP requests duration (seconds).",
	}, []string{"code"})

	DBConnectionFailed = promauto.With(PromRegistry).NewCounterVec(prometheus.CounterOpts{
		Name: "search_api_db_connection_failed_total",
		Help: "The total number of DB connections that has failed.",
	}, []string{"route"})

	DBQueryDuration = promauto.With(PromRegistry).NewHistogramVec(prometheus.HistogramOpts{
		Name: "search_api_dbquery_duration",
		Help: "Histogram of outbound DB query latency (seconds).",
	}, []string{"query_name"})
)
