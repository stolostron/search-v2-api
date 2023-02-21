// Copyright Contributors to the Open Cluster Management project
package metric

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HttpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "search_http_duration_seconds",
		Help: "Latency of single HTTP request in (milli)seconds.",
	}, []string{"path", "method"})
)

var (
	HttpRequestTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "search_http_total",
		Help: "Total number HTTP requests.",
	}, []string{"path", "method"})
)

var (
	AuthnFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "search_authn_failed_total",
		Help: "The total number of authentication requests that has failed",
	}, []string{"reason"})
)

var (
	AuthzFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "search_authz_failed_total",
		Help: "The total number of authorization requests that has failed",
	}, []string{"reason"})
)

var (
	DBConnectionFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "search_db_connection_failed_total",
		Help: "The total number of DB connection that has failed",
	}, []string{"route"})
)

var (
	DBConnectionSuccess = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "search_db_connection_success_total",
		Help: "The total number of DB connection that has succeeded",
	}, []string{"route"})
)

var (
	DBQueryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "search_dbquery_duration_seconds",
		Help: "Latency of DB requests in seconds.",
	}, []string{"query"})
)

var (
	DBQueryBuildDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "search_dbquery_build_duration_seconds",
		Help: "Latency of DB query build in seconds.",
	}, []string{"query"})
)

// var (
// 	UserSessionDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
// 		Name: "search_user_session_duration_seconds",
// 		Help: "Total time of session partitioned by user.",
// 	}, []string{"userid"})
// )
