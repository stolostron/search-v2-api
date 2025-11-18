// Copyright Contributors to the Open Cluster Management project
package server

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/google/uuid"
	"github.com/stolostron/search-v2-api/pkg/metrics"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	"k8s.io/klog/v2"
)

// ContextKey type for WebSocket-specific context keys
type wsContextKey string

const (
	// Context keys for WebSocket connections
	wsContextKeyConnectionID  wsContextKey = "ws-connection-id"
	wsContextKeyConnectedAt   wsContextKey = "ws-connected-at"
	wsContextKeyAuthenticated wsContextKey = "ws-authenticated"

	// WSConnectionIDKey is the exported string constant for the connection ID context key
	// Use this in other packages to avoid import cycles: ctx.Value("ws-connection-id")
	WSConnectionIDKey = string(wsContextKeyConnectionID)
)

// Connection tracking
var (
	activeConnections      = make(map[string]*wsConnectionInfo)
	activeConnectionsMutex sync.RWMutex
)

// wsConnectionInfo stores metadata about a WebSocket connection
type wsConnectionInfo struct {
	ID          string
	ConnectedAt time.Time
	AuthToken   string
}

// WebSocketInitFunc creates the initialization function for WebSocket connections
// This function intercepts the WebSocket connection initialization and:
// 1. Authenticates the connection using the token from the init payload
// 2. Tracks active connections
// 3. Adds metadata to the context
// 4. Records metrics
func WebSocketInitFunc() func(context.Context, transport.InitPayload) (context.Context, *transport.InitPayload, error) {
	return func(ctx context.Context, initPayload transport.InitPayload) (context.Context, *transport.InitPayload, error) {
		// Generate unique connection ID
		connectionID := uuid.New().String()[:8]
		connectedAt := time.Now()
		klog.V(2).Infof("Initializing WebSocket connection [%s] ", connectionID)

		// Increment total connections counter
		metrics.WebSocketConnectionsTotal.Inc()

		// Extract authentication token from the payload
		// The GraphQL WebSocket protocol typically sends the token in the connection_init payload
		authToken, err := extractAuthToken(initPayload)
		if err != nil {
			klog.Warningf("[%s] WebSocket connection rejected: %v", connectionID, err)
			metrics.WebSocketConnectionsFailed.WithLabelValues("missing_token").Inc()
			return ctx, nil, fmt.Errorf("authentication required: %w", err)
		}

		// Validate the token using the existing RBAC cache
		authenticated, err := rbac.GetCache().IsValidToken(ctx, authToken)
		if err != nil {
			klog.Warningf("[%s] WebSocket authentication error: %v", connectionID, err)
			metrics.WebSocketConnectionsFailed.WithLabelValues("auth_error").Inc()
			return ctx, nil, fmt.Errorf("authentication error: %w", err)
		}

		if !authenticated {
			klog.Warningf("[%s] WebSocket connection rejected: invalid token", connectionID)
			metrics.WebSocketConnectionsFailed.WithLabelValues("invalid_token").Inc()
			return ctx, nil, fmt.Errorf("invalid authentication token")
		}

		klog.V(4).Infof("WebSocket connection [%s] authentication successful", connectionID)

		// Store connection info
		connInfo := &wsConnectionInfo{
			ID:          connectionID,
			ConnectedAt: connectedAt,
			AuthToken:   authToken,
		}

		activeConnectionsMutex.Lock()
		activeConnections[connectionID] = connInfo
		activeConnectionsMutex.Unlock()

		// Update metrics
		metrics.SubscriptionsActive.Inc()

		// Log connection details
		klog.Infof("WebSocket connection [%s] established - Active connections: %d",
			connectionID, len(activeConnections))

		// Add metadata to context
		ctx = context.WithValue(ctx, wsContextKeyConnectionID, connectionID)
		ctx = context.WithValue(ctx, wsContextKeyConnectedAt, connectedAt)
		ctx = context.WithValue(ctx, wsContextKeyAuthenticated, true)

		// Add the auth token to context using the same key as the HTTP middleware
		// This ensures subscription resolvers can access the token the same way
		ctx = context.WithValue(ctx, rbac.ContextAuthTokenKey, authToken)

		// Return the modified context and payload
		return ctx, &initPayload, nil
	}
}

// WebSocketCloseFunc creates the close function for WebSocket connections
// This function is called when a WebSocket connection closes and:
// 1. Removes the connection from tracking
// 2. Records connection duration metrics
// 3. Logs the disconnection
func WebSocketCloseFunc() func(context.Context, int) {
	return func(ctx context.Context, closeCode int) {
		connectionID := getConnectionID(ctx)
		klog.V(4).Infof("Closing WebSocket connection [%s]", connectionID)

		// Get connection info for metrics
		activeConnectionsMutex.Lock()
		connInfo, exists := activeConnections[connectionID]
		if exists {
			delete(activeConnections, connectionID)
		}
		activeConnectionsMutex.Unlock()

		// Update metrics
		metrics.SubscriptionsActive.Dec()

		if exists {
			// Record connection duration
			duration := time.Since(connInfo.ConnectedAt)
			metrics.SubscriptionDuration.Observe(duration.Seconds())

			klog.V(2).Infof("WebSocket connection [%s] closed - Code: %d, Duration: %v, Active connections: %d",
				connectionID, closeCode, duration, len(activeConnections))
		} else {
			klog.Warningf("WebSocket connection [%s] closed - Code: %d (connection not tracked)",
				connectionID, closeCode)
		}
	}
}

// extractAuthToken extracts the Authorization token from the init payload
func extractAuthToken(payload transport.InitPayload) (string, error) {
	if val, ok := payload["Authorization"]; ok {
		if token, ok := val.(string); ok && token != "" {
			// Remove "Bearer " prefix if present
			token = strings.TrimSpace(token)
			token = strings.TrimPrefix(token, "Bearer")
			token = strings.TrimPrefix(token, "bearer")
			token = strings.TrimSpace(token)
			klog.Infof("Extracted Authorization token: [%s]", token)
			if token != "" {
				klog.V(5).Infof("Found Authorization token in connection payload.")
				return token, nil
			}
		}
	}
	return "", fmt.Errorf("no Authorization token found in connection payload")
}

// getConnectionID retrieves the connection ID from context
func getConnectionID(ctx context.Context) string {
	if id, ok := ctx.Value(wsContextKeyConnectionID).(string); ok {
		return id
	}
	return "unknown"
}

// WebSocketErrorFunc creates the error function for WebSocket connections
// This function is called when a WebSocket connection error occurs and:
// 1. Logs the error
// 2. Records error metrics
func WebSocketErrorFunc() func(context.Context, error) {
	return func(ctx context.Context, err error) {
		connectionID := getConnectionID(ctx)
		klog.Errorf("WebSocket connection [%s] error: %v", connectionID, err)
	}
}
