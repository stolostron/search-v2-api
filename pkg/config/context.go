// Copyright Contributors to the Open Cluster Management project
package config

// ContextKey type for WebSocket-specific context keys
type wsContextKey string

const (
	// Context keys for WebSocket connections
	WSContextKeyConnectionID  wsContextKey = "ws-connection-id"
	WSContextKeyConnectedAt   wsContextKey = "ws-connected-at"
	WSContextKeyAuthenticated wsContextKey = "ws-authenticated"
)
