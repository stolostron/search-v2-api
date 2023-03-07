// Copyright Contributors to the Open Cluster Management project
package metric

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// HistogramVec - collector that bundles a set of observations that share Desc but have different values for their variable labels
// Used when we want to count the same thing partitioned by some dimension(s) ex. http request latencies broken up by status code and method.
var (
	HttpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "search_http_duration_seconds",
		Help: "Latency of single HTTP request",
	}, []string{"status_code", "action"})
)

// http := prometheus.NewHistogramVec(prometheus.HistogramOpts{
// 	Namespace: "http",
// 	Name:      "request_duration_seconds",
// 	Help:      "The latency of the HTTP requests.",
// 	Buckets:   prometheus.DefBuckets,
// }, []string{"handler", "method", "code"})

// var (
// 	ResponseStatus = promauto.NewCounterVec(prometheus.CounterOpts{
// 		Name: "response_status",
// 		Help: "Status of HTTP response",
// 	}, []string{"status"})
// )

var (
	HttpRequestTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "search_http_total",
		Help: "Total number HTTP requests.",
	}, []string{"path", "method"})
)

//we can use curry with for these two below to slice HttpDuration metric by label authen/author
var (
	AuthnFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "search_authn_failed_total",
		Help: "The total number of authentication requests that has failed",
	}, []string{"status_code"})
)

var (
	AuthzFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "search_authz_failed_total",
		Help: "The total number of authorization requests that has failed",
	}, []string{"status_code"})
)

var (
	DBConnectionFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "search_db_connection_failed_total",
		Help: "The total number of DB connection that has failed",
	}, []string{"route"})
)

// Helper function to curry a metric with pre-defined labels
func HttpDurationByLabels(labels prometheus.Labels) *prometheus.HistogramVec {
	return HttpDuration.MustCurryWith(labels).(*prometheus.HistogramVec)
}
