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
	}, []string{"code", "remoteAddr", "userAgent"})

	DBConnectionFailed = promauto.With(PromRegistry).NewCounter(prometheus.CounterOpts{
		Name: "search_api_db_connection_failed",
		Help: "The number of failed database connection attempts.",
	})

	DBQueryDuration = promauto.With(PromRegistry).NewHistogramVec(prometheus.HistogramOpts{
		Name: "search_api_db_query_duration",
		Help: "Latency (seconds) for database queries.",
	}, []string{"query_name"})

	// Subscription metrics (WebSockets)
	SubscriptionsActive = promauto.With(PromRegistry).NewGauge(prometheus.GaugeOpts{
		Name: "search_api_subscriptions_active",
		Help: "The number of active subscriptions.",
	})

	SubscriptionDuration = promauto.With(PromRegistry).NewHistogram(prometheus.HistogramOpts{
		Name: "search_api_subscription_duration",
		Help: "Duration (seconds) of subscriptions.",
	})

	WebSocketConnectionsTotal = promauto.With(PromRegistry).NewCounter(prometheus.CounterOpts{
		Name: "search_api_websocket_connections_total",
		Help: "The total number of WebSocket connection attempts.",
	})

	WebSocketConnectionsFailed = promauto.With(PromRegistry).NewCounterVec(prometheus.CounterOpts{
		Name: "search_api_websocket_connections_failed",
		Help: "The number of failed WebSocket connection attempts.",
	}, []string{"reason"})
)
