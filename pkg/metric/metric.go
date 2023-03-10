// Copyright Contributors to the Open Cluster Management project
package metric

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HttpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "search_http_duration_seconds",
		Help: "Latency of of HTTP requests in seconds.",
	}, []string{"code"})
)

var (
	DBConnectionFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "search_db_connection_failed_total",
		Help: "The total number of DB connection that has failed",
	}, []string{"route"})
)

var (
	DBQueryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "search_dbquery_duration_seconds",
		Help: "Latency of DB requests in seconds.",
	}, []string{"query_name"})
)
