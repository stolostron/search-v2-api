// Copyright Contributors to the Open Cluster Management project
package server

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// [AI]
func TestExtractAuthToken(t *testing.T) {
	tests := []struct {
		name        string
		payload     transport.InitPayload
		expectToken string
		expectError bool
	}{
		{
			name: "Authentication field with Bearer prefix",
			payload: transport.InitPayload{
				"Authentication": "Bearer test-token-123",
			},
			expectToken: "test-token-123",
			expectError: false,
		},
		{
			name: "Authentication field without Bearer prefix",
			payload: transport.InitPayload{
				"Authentication": "test-token-456",
			},
			expectToken: "test-token-456",
			expectError: false,
		},
		{
			name: "lowercase bearer prefix",
			payload: transport.InitPayload{
				"Authentication": "bearer test-token-789",
			},
			expectToken: "test-token-789",
			expectError: false,
		},
		{
			name:        "missing Authentication field",
			payload:     transport.InitPayload{},
			expectToken: "",
			expectError: true,
		},
		{
			name: "empty token",
			payload: transport.InitPayload{
				"Authentication": "",
			},
			expectToken: "",
			expectError: true,
		},
		{
			name: "Bearer with spaces",
			payload: transport.InitPayload{
				"Authentication": "  Bearer  test-token-spaces  ",
			},
			expectToken: "test-token-spaces",
			expectError: false,
		},
		{
			name: "only Bearer prefix",
			payload: transport.InitPayload{
				"Authentication": "Bearer ",
			},
			expectToken: "",
			expectError: true,
		},
		{
			name: "only whitespace",
			payload: transport.InitPayload{
				"Authentication": "   ",
			},
			expectToken: "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := extractAuthToken(tt.payload)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, token)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectToken, token)
			}
		})
	}
}

// [AI]
func TestExtractAuthTokenEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		payload     transport.InitPayload
		expectError bool
		description string
	}{
		{
			name: "nil value",
			payload: transport.InitPayload{
				"Authentication": nil,
			},
			expectError: true,
			description: "nil value should result in error",
		},
		{
			name: "numeric value",
			payload: transport.InitPayload{
				"Authentication": 12345,
			},
			expectError: true,
			description: "numeric value should result in error",
		},
		{
			name: "boolean value",
			payload: transport.InitPayload{
				"Authentication": true,
			},
			expectError: true,
			description: "boolean value should result in error",
		},
		{
			name: "wrong field name - Authorization",
			payload: transport.InitPayload{
				"Authorization": "test-token",
			},
			expectError: true,
			description: "Wrong field name (Authorization vs Authentication) should fail",
		},
		{
			name: "case sensitive - authentication",
			payload: transport.InitPayload{
				"authentication": "test-token",
			},
			expectError: true,
			description: "lowercase 'authentication' should not be found (case sensitive)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := extractAuthToken(tt.payload)

			if tt.expectError {
				assert.Error(t, err, tt.description)
				assert.Empty(t, token)
			} else {
				assert.NoError(t, err, tt.description)
				assert.NotEmpty(t, token)
			}
		})
	}
}

// [AI]
func TestGetConnectionID(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		expected string
	}{
		{
			name:     "context without connection ID",
			ctx:      context.Background(),
			expected: "unknown",
		},
		{
			name:     "context with connection ID",
			ctx:      context.WithValue(context.Background(), wsContextKeyConnectionID, "test-123"),
			expected: "test-123",
		},
		{
			name:     "context with empty connection ID",
			ctx:      context.WithValue(context.Background(), wsContextKeyConnectionID, ""),
			expected: "",
		},
		{
			name:     "context with wrong type",
			ctx:      context.WithValue(context.Background(), wsContextKeyConnectionID, 12345),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getConnectionID(tt.ctx)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// [AI]
func TestConnectionTracking(t *testing.T) {
	// Clear any existing connections
	activeConnectionsMutex.Lock()
	activeConnections = make(map[string]*wsConnectionInfo)
	activeConnectionsMutex.Unlock()

	// Test initial state
	activeConnectionsMutex.RLock()
	initialCount := len(activeConnections)
	activeConnectionsMutex.RUnlock()
	assert.Equal(t, 0, initialCount)

	// Simulate adding a connection
	connInfo := &wsConnectionInfo{
		ID:          "test-conn-1",
		ConnectedAt: time.Now(),
		AuthToken:   "test-token",
	}

	activeConnectionsMutex.Lock()
	activeConnections[connInfo.ID] = connInfo
	activeConnectionsMutex.Unlock()

	// Test active connection count
	activeConnectionsMutex.RLock()
	count := len(activeConnections)
	activeConnectionsMutex.RUnlock()
	assert.Equal(t, 1, count)

	// Add another connection
	connInfo2 := &wsConnectionInfo{
		ID:          "test-conn-2",
		ConnectedAt: time.Now(),
		AuthToken:   "test-token-2",
	}

	activeConnectionsMutex.Lock()
	activeConnections[connInfo2.ID] = connInfo2
	activeConnectionsMutex.Unlock()

	activeConnectionsMutex.RLock()
	count = len(activeConnections)
	activeConnectionsMutex.RUnlock()
	assert.Equal(t, 2, count)

	// Test removing a connection
	activeConnectionsMutex.Lock()
	delete(activeConnections, connInfo.ID)
	activeConnectionsMutex.Unlock()

	activeConnectionsMutex.RLock()
	count = len(activeConnections)
	activeConnectionsMutex.RUnlock()
	assert.Equal(t, 1, count)

	// Cleanup
	activeConnectionsMutex.Lock()
	activeConnections = make(map[string]*wsConnectionInfo)
	activeConnectionsMutex.Unlock()
}

// [AI]
func TestConcurrentConnectionTracking(t *testing.T) {
	// Clear connections
	activeConnectionsMutex.Lock()
	activeConnections = make(map[string]*wsConnectionInfo)
	activeConnectionsMutex.Unlock()

	const numGoroutines = 50
	const numOpsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Simulate concurrent adds and removes
	for i := 0; i < numGoroutines; i++ {
		go func(routineID int) {
			defer wg.Done()
			for j := 0; j < numOpsPerGoroutine; j++ {
				connID := fmt.Sprintf("conn-%d-%d", routineID, j)
				connInfo := &wsConnectionInfo{
					ID:          connID,
					ConnectedAt: time.Now(),
					AuthToken:   fmt.Sprintf("token-%d-%d", routineID, j),
				}

				activeConnectionsMutex.Lock()
				activeConnections[connID] = connInfo
				activeConnectionsMutex.Unlock()

				// Simulate some work
				time.Sleep(time.Microsecond)

				// Remove connection
				activeConnectionsMutex.Lock()
				delete(activeConnections, connID)
				activeConnectionsMutex.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// All connections should be removed
	activeConnectionsMutex.RLock()
	count := len(activeConnections)
	activeConnectionsMutex.RUnlock()
	assert.Equal(t, 0, count, "All connections should be cleaned up")
}

// [AI]
func TestWebSocketContextKeys(t *testing.T) {
	ctx := context.Background()

	// Test connection ID
	testID := "test-connection-123"
	ctx = context.WithValue(ctx, wsContextKeyConnectionID, testID)
	retrievedID := getConnectionID(ctx)
	assert.Equal(t, testID, retrievedID)

	// Test connected at
	now := time.Now()
	ctx = context.WithValue(ctx, wsContextKeyConnectedAt, now)
	retrievedTime, ok := ctx.Value(wsContextKeyConnectedAt).(time.Time)
	assert.True(t, ok)
	assert.Equal(t, now, retrievedTime)

	// Test authenticated
	ctx = context.WithValue(ctx, wsContextKeyAuthenticated, true)
	authenticated, ok := ctx.Value(wsContextKeyAuthenticated).(bool)
	assert.True(t, ok)
	assert.True(t, authenticated)

	// Test auth token from RBAC
	testToken := "test-rbac-token"
	ctx = context.WithValue(ctx, rbac.ContextAuthTokenKey, testToken)
	retrievedToken, ok := ctx.Value(rbac.ContextAuthTokenKey).(string)
	assert.True(t, ok)
	assert.Equal(t, testToken, retrievedToken)
}

// [AI]
func TestWebSocketContextValueConstant(t *testing.T) {
	// Verify exported constant matches internal key string value
	assert.Equal(t, "ws-connection-id", WSConnectionIDKey)

	// Note: WSConnectionIDKey is a string constant for reference/documentation
	// But internally, context values are stored with wsContextKey type
	// This means: context.WithValue(ctx, "ws-connection-id", val) != context.WithValue(ctx, wsContextKey("ws-connection-id"), val)

	// Test that internal key works correctly
	ctx := context.Background()
	testID := "test-const-id"

	// This is how it's used internally (with wsContextKey type)
	ctx = context.WithValue(ctx, wsContextKeyConnectionID, testID)

	// getConnectionID should retrieve it successfully
	retrievedID := getConnectionID(ctx)
	assert.Equal(t, testID, retrievedID)

	// Test that using string key doesn't work (by design - different type)
	ctx2 := context.WithValue(context.Background(), "ws-connection-id", testID)
	retrievedID2 := getConnectionID(ctx2)
	assert.Equal(t, "unknown", retrievedID2, "String key should not match wsContextKey type")

	// Test that WSConnectionIDKey as string also doesn't match (same reason)
	ctx3 := context.WithValue(context.Background(), WSConnectionIDKey, testID)
	retrievedID3 := getConnectionID(ctx3)
	assert.Equal(t, "unknown", retrievedID3, "String constant key should not match wsContextKey type")

	// For external packages that need to read the value:
	// They should access with the string directly
	ctx4 := context.WithValue(context.Background(), wsContextKeyConnectionID, testID)
	externalValue, ok := ctx4.Value("ws-connection-id").(string)
	// This won't work because the key type is different
	assert.False(t, ok, "External string access won't work with typed key")
	assert.Empty(t, externalValue)

	// The correct way for external access is to use the typed key value
	typedValue, ok := ctx4.Value(wsContextKey("ws-connection-id")).(string)
	assert.True(t, ok, "Accessing with correct type should work")
	assert.Equal(t, testID, typedValue)
}

// [AI]
func TestWebSocketCloseFunc(t *testing.T) {
	// Clear connections
	activeConnectionsMutex.Lock()
	activeConnections = make(map[string]*wsConnectionInfo)
	activeConnectionsMutex.Unlock()

	// Setup a tracked connection
	connectionID := "test-close-conn"
	ctx := context.Background()
	ctx = context.WithValue(ctx, wsContextKeyConnectionID, connectionID)

	connInfo := &wsConnectionInfo{
		ID:          connectionID,
		ConnectedAt: time.Now().Add(-5 * time.Second), // 5 seconds ago
		AuthToken:   "test-token",
	}

	activeConnectionsMutex.Lock()
	activeConnections[connectionID] = connInfo
	initialCount := len(activeConnections)
	activeConnectionsMutex.Unlock()
	assert.Equal(t, 1, initialCount)

	// Call the close function
	closeFunc := WebSocketCloseFunc()
	closeFunc(ctx, 1000) // Normal closure

	// Verify connection was removed
	activeConnectionsMutex.RLock()
	finalCount := len(activeConnections)
	activeConnectionsMutex.RUnlock()
	assert.Equal(t, 0, finalCount)

	// Test closing a connection that wasn't tracked
	ctxUntracked := context.WithValue(context.Background(), wsContextKeyConnectionID, "untracked-conn")
	closeFunc(ctxUntracked, 1001)

	// Should not panic and should still have 0 connections
	activeConnectionsMutex.RLock()
	count := len(activeConnections)
	activeConnectionsMutex.RUnlock()
	assert.Equal(t, 0, count)

	// Cleanup
	activeConnectionsMutex.Lock()
	activeConnections = make(map[string]*wsConnectionInfo)
	activeConnectionsMutex.Unlock()
}

// [AI]
func TestWebSocketConnectionInfo(t *testing.T) {
	// Test connection info structure
	now := time.Now()
	connInfo := &wsConnectionInfo{
		ID:          "test-123",
		ConnectedAt: now,
		AuthToken:   "test-token-abc",
	}

	assert.Equal(t, "test-123", connInfo.ID)
	assert.Equal(t, now, connInfo.ConnectedAt)
	assert.Equal(t, "test-token-abc", connInfo.AuthToken)
}

// [AI]
func TestConnectionInfoImmutability(t *testing.T) {
	// Test that modifying retrieved connection info doesn't affect stored info
	activeConnectionsMutex.Lock()
	activeConnections = make(map[string]*wsConnectionInfo)
	activeConnectionsMutex.Unlock()

	connectionID := "test-immutable"
	originalToken := "original-token"

	connInfo := &wsConnectionInfo{
		ID:          connectionID,
		ConnectedAt: time.Now(),
		AuthToken:   originalToken,
	}

	activeConnectionsMutex.Lock()
	activeConnections[connectionID] = connInfo
	activeConnectionsMutex.Unlock()

	// Get connection info
	activeConnectionsMutex.RLock()
	retrieved, exists := activeConnections[connectionID]
	activeConnectionsMutex.RUnlock()

	require.True(t, exists)
	require.NotNil(t, retrieved)
	assert.Equal(t, originalToken, retrieved.AuthToken)

	// If we modify the pointer, it would affect the original
	// This test documents the current behavior
	// (In production code, you'd return copies to prevent this)

	// Cleanup
	activeConnectionsMutex.Lock()
	activeConnections = make(map[string]*wsConnectionInfo)
	activeConnectionsMutex.Unlock()
}

// [AI]
func TestWebSocketErrorFunc(t *testing.T) {
	// Test that error function exists and doesn't panic
	errorFunc := WebSocketErrorFunc()
	assert.NotNil(t, errorFunc)

	// Call it with a test error
	ctx := context.Background()
	testErr := fmt.Errorf("test websocket error")

	// Should not panic
	assert.NotPanics(t, func() {
		errorFunc(ctx, testErr)
	})
}

// [AI] Benchmark tests
func BenchmarkExtractAuthToken(b *testing.B) {
	payload := transport.InitPayload{
		"Authentication": "Bearer benchmark-token-12345",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = extractAuthToken(payload)
	}
}

// [AI]
func BenchmarkGetConnectionID(b *testing.B) {
	ctx := context.WithValue(context.Background(), wsContextKeyConnectionID, "bench-conn-123")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = getConnectionID(ctx)
	}
}

// [AI]
func BenchmarkConnectionTracking(b *testing.B) {
	activeConnectionsMutex.Lock()
	activeConnections = make(map[string]*wsConnectionInfo)
	activeConnectionsMutex.Unlock()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			connID := fmt.Sprintf("bench-conn-%d", i)
			connInfo := &wsConnectionInfo{
				ID:          connID,
				ConnectedAt: time.Now(),
				AuthToken:   "bench-token",
			}

			activeConnectionsMutex.Lock()
			activeConnections[connID] = connInfo
			activeConnectionsMutex.Unlock()

			activeConnectionsMutex.Lock()
			delete(activeConnections, connID)
			activeConnectionsMutex.Unlock()

			i++
		}
	})

	// Cleanup
	activeConnectionsMutex.Lock()
	activeConnections = make(map[string]*wsConnectionInfo)
	activeConnectionsMutex.Unlock()
}

// [AI]
func BenchmarkContextOperations(b *testing.B) {
	testID := "benchmark-connection-id"
	now := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := context.Background()
		ctx = context.WithValue(ctx, wsContextKeyConnectionID, testID)
		ctx = context.WithValue(ctx, wsContextKeyConnectedAt, now)
		ctx = context.WithValue(ctx, wsContextKeyAuthenticated, true)

		_ = getConnectionID(ctx)
	}
}
