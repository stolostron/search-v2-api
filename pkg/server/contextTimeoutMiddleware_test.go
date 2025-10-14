// Copyright Contributors to the Open Cluster Management project

package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stretchr/testify/assert"
)

// [AI] Test that regular (non-websocket) HTTP requests get the RequestTimeout applied
func TestRequestTimeout_RegularRequest(t *testing.T) {
	cfg := config.Config{
		RequestTimeout:       5000,  // 5 seconds
		StreamRequestTimeout: 10000, // 10 seconds
	}

	// Create a handler that captures the request context
	var capturedCtx context.Context
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with the middleware
	middleware := RequestTimeout(cfg)
	handler := middleware(nextHandler)

	// Create a regular HTTP request (not a websocket upgrade)
	req := httptest.NewRequest("POST", "/searchapi/graphql", nil)
	rr := httptest.NewRecorder()

	// Execute the request
	handler.ServeHTTP(rr, req)

	// Verify the next handler was called
	assert.Equal(t, http.StatusOK, rr.Code, "Expected status %d, got %d", http.StatusOK, rr.Code)

	// Verify the context has a deadline
	assert.NotNil(t, capturedCtx, "Context was not captured")

	deadline, ok := capturedCtx.Deadline()
	assert.True(t, ok, "Expected context to have a deadline, but it doesn't")

	// Verify the deadline is approximately 5 seconds from now with some tolerance for test execution time
	expectedDeadline := time.Now().Add(time.Duration(cfg.RequestTimeout) * time.Millisecond)
	timeDiff := deadline.Sub(expectedDeadline).Abs()
	assert.LessOrEqual(t, timeDiff, 1*time.Second, "Expected deadline around %v, got %v (diff: %v)", expectedDeadline, deadline, timeDiff)
}

// [AI] Test that websocket requests get the StreamRequestTimeout applied
func TestRequestTimeout_WebSocketRequest(t *testing.T) {
	cfg := config.Config{
		RequestTimeout:       5000,  // 5 seconds
		StreamRequestTimeout: 10000, // 10 seconds
	}

	// Create a handler that captures the request context
	var capturedCtx context.Context
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with the middleware
	middleware := RequestTimeout(cfg)
	handler := middleware(nextHandler)

	// Create a websocket upgrade request
	req := httptest.NewRequest("GET", "/searchapi/graphql", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")

	rr := httptest.NewRecorder()

	// Execute the request
	handler.ServeHTTP(rr, req)

	// Verify the next handler was called
	assert.Equal(t, http.StatusOK, rr.Code, "Expected status %d, got %d", http.StatusOK, rr.Code)

	// Verify the context has a deadline
	assert.NotNil(t, capturedCtx, "Context was not captured")

	deadline, ok := capturedCtx.Deadline()
	assert.True(t, ok, "Expected context to have a deadline, but it doesn't")

	// Verify the deadline is approximately 10 seconds from now (StreamRequestTimeout) with some tolerance for test execution time
	expectedDeadline := time.Now().Add(time.Duration(cfg.StreamRequestTimeout) * time.Millisecond)
	timeDiff := deadline.Sub(expectedDeadline).Abs()
	assert.LessOrEqual(t, timeDiff, 1*time.Second, "Expected deadline around %v, got %v (diff: %v)", expectedDeadline, deadline, timeDiff)
}

// [AI] Test that the context timeout actually expires for regular (non-websocket) HTTP requests
func TestRequestTimeout_ContextExpires(t *testing.T) {
	cfg := config.Config{
		RequestTimeout:       10, // 10ms
		StreamRequestTimeout: 20, // 20ms
	}

	// Create a handler that sleeps longer than the timeout
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep for longer than the timeout
		time.Sleep(time.Duration(cfg.RequestTimeout+1) * time.Millisecond)

		// Check if context expired
		select {
		case <-r.Context().Done():
			assert.Equal(t, context.DeadlineExceeded, r.Context().Err(), "Expected context.DeadlineExceeded, got %v", r.Context().Err())
		default:
			assert.Fail(t, "Expected context to be done, but it wasn't")
		}

		w.WriteHeader(http.StatusOK)
	})

	// Wrap with the middleware
	middleware := RequestTimeout(cfg)
	handler := middleware(nextHandler)

	// Create a regular HTTP request
	req := httptest.NewRequest("POST", "/searchapi/graphql", nil)
	rr := httptest.NewRecorder()

	// Execute the request
	handler.ServeHTTP(rr, req)
}

// [AI] Test that context timeout expires for websocket requests
func TestRequestTimeout_WebSocketContextExpires(t *testing.T) {
	cfg := config.Config{
		RequestTimeout:       10, // 10ms
		StreamRequestTimeout: 20, // 20ms
	}

	// Create a handler that sleeps longer than the stream timeout
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep for longer than the stream timeout
		time.Sleep(time.Duration(cfg.StreamRequestTimeout+1) * time.Millisecond)

		// Check if context expired
		select {
		case <-r.Context().Done():
			assert.Equal(t, context.DeadlineExceeded, r.Context().Err(), "Expected context.DeadlineExceeded, got %v", r.Context().Err())
		default:
			assert.Fail(t, "Expected context to be done, but it wasn't")
		}

		w.WriteHeader(http.StatusOK)
	})

	// Wrap with the middleware
	middleware := RequestTimeout(cfg)
	handler := middleware(nextHandler)

	// Create a websocket upgrade request
	req := httptest.NewRequest("GET", "/searchapi/graphql", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")

	rr := httptest.NewRecorder()

	// Execute the request
	handler.ServeHTTP(rr, req)
}

// [AI] Test that multiple requests get independent contexts
func TestRequestTimeout_IndependentContexts(t *testing.T) {
	cfg := config.Config{
		RequestTimeout:       5000,
		StreamRequestTimeout: 10000,
	}

	// Track captured contexts
	var contexts []context.Context
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contexts = append(contexts, r.Context())
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with the middleware
	middleware := RequestTimeout(cfg)
	handler := middleware(nextHandler)

	// Create and execute two requests
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/searchapi/graphql", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	// Verify we captured two contexts
	assert.Len(t, contexts, 2, "Expected 2 contexts")

	// Verify the contexts are different instances
	assert.NotEqual(t, contexts[0], contexts[1], "Expected independent contexts, but they are the same instance")

	// Verify both have deadlines
	for i, ctx := range contexts {
		_, ok := ctx.Deadline()
		assert.True(t, ok, "Context %d should have a deadline", i)
	}
}
