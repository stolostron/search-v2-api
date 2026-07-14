// Copyright Contributors to the Open Cluster Management project

package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestRequestTimeoutReturnsUnavailableIfTimeout verifies that the middleware
// returns 503 when the downstream handler exceeds the configured timeout.
//
// The handler blocks until its request context is cancelled.  http.TimeoutHandler
// cancels that context exactly when the deadline fires, so the handler always
// finishes *after* the timeout — the result is deterministic regardless of
// scheduler latency.
func TestRequestTimeoutReturnsUnavailableIfTimeout(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until the timeout middleware cancels our context, then return
		// immediately.  This guarantees the timeout always fires first.
		<-r.Context().Done()
	})
	handler := TimeoutHandler(50 * time.Millisecond)

	req := httptest.NewRequest("POST", "/searchapi/graphql", nil)
	rr := httptest.NewRecorder()

	handler(nextHandler).ServeHTTP(rr, req)
	var body bytes.Buffer
	actual := rr.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(actual.Body)
	_, err := io.Copy(&body, actual.Body)

	assert.Nil(t, err, "Expected nil error reading response body")
	assert.Equal(t, "Request timed out", body.String())
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
}

func TestRequestTimeoutReturnsOKIfNoTimeout(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no-op — completes instantly, well within the timeout
	})
	handler := TimeoutHandler(50 * time.Millisecond)

	req := httptest.NewRequest("POST", "/searchapi/graphql", nil)
	rr := httptest.NewRecorder()

	handler(nextHandler).ServeHTTP(rr, req)
	var body bytes.Buffer
	actual := rr.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(actual.Body)
	_, err := io.Copy(&body, actual.Body)

	assert.Nil(t, err, "Expected nil error reading response body")
	assert.Equal(t, "", body.String())
	assert.Equal(t, http.StatusOK, rr.Code)
}

// TestRequestTimeoutReturnsUnavailableIfContextTimeout verifies that a
// pre-cancelled request context causes a 503 even when the handler is a no-op.
// http.TimeoutHandler checks the context before dispatching; a cancelled context
// is treated as an already-expired timeout.
func TestRequestTimeoutReturnsUnavailableIfContextTimeout(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no-op
	})
	handler := TimeoutHandler(50 * time.Millisecond)

	req := httptest.NewRequest("POST", "/searchapi/graphql", nil)
	rr := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(context.TODO())
	cancel() // cancel before the request is handled
	handler(nextHandler).ServeHTTP(rr, req.WithContext(ctx))

	var body bytes.Buffer
	actual := rr.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(actual.Body)
	_, err := io.Copy(&body, actual.Body)

	assert.Nil(t, err, "Expected nil error reading response body")
	// The body is empty because the context was cancelled externally (not by our
	// timeout handler), so http.TimeoutHandler writes no timeout message.
	assert.Equal(t, "", body.String())
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
}

// TestTimeoutMiddleware_SkipsWebSocketUpgrades verifies that the middleware is
// bypassed for WebSocket upgrade requests.  http.TimeoutHandler wraps the
// ResponseWriter in a way that does not implement http.Hijacker, which is
// required for the WebSocket handshake.
func TestTimeoutMiddleware_SkipsWebSocketUpgrades(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no-op
	})
	handler := TimeoutHandler(50 * time.Millisecond)

	req := httptest.NewRequest("GET", "/searchapi/graphql", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	res := httptest.NewRecorder()

	handler(nextHandler).ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
}
