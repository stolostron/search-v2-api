// Copyright Contributors to the Open Cluster Management project

package server

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRequestTimeoutReturnsUnavailableIfTimeout verifies that the middleware
// returns 503 when the downstream handler exceeds the configured timeout.
//
// The handler blocks on a release channel that is never closed, so it cannot
// return before ServeHTTP does.  This removes even the theoretical race in
// http.TimeoutHandler's internal select between the timer branch and the
// handler-done branch: the handler goroutine is permanently stuck until after
// ServeHTTP returns, so the timer is the only branch that can fire.
func TestRequestTimeoutReturnsUnavailableIfTimeout(t *testing.T) {
	release := make(chan struct{}) // never closed; handler blocks forever
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
			// context cancelled by the timeout — still don't return yet;
			// wait for the release so the select above is truly the only exit.
			<-release
		}
	})
	handler := TimeoutHandler(50 * time.Millisecond)

	req := httptest.NewRequest("POST", "/searchapi/graphql", nil)
	rr := httptest.NewRecorder()

	handler(nextHandler).ServeHTTP(rr, req)
	// ServeHTTP has returned — close release so the handler goroutine can exit
	// and be garbage-collected cleanly.
	close(release)

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
// pre-cancelled request context causes a 503 even when the handler is
// permanently blocked.  http.TimeoutHandler selects on the context's Done
// channel; a pre-cancelled context means Done is already closed before
// ServeHTTP is called, so the timeout branch wins immediately.
func TestRequestTimeoutReturnsUnavailableIfContextTimeout(t *testing.T) {
	release := make(chan struct{}) // never closed during the call
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // block unconditionally so the handler can never win the select
	})
	handler := TimeoutHandler(50 * time.Millisecond)

	req := httptest.NewRequest("POST", "/searchapi/graphql", nil)
	rr := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(context.TODO())
	cancel() // cancel before the request is handled
	handler(nextHandler).ServeHTTP(rr, req.WithContext(ctx))
	close(release) // let the handler goroutine exit

	var body bytes.Buffer
	actual := rr.Result()
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(actual.Body)
	_, err := io.Copy(&body, actual.Body)

	assert.Nil(t, err, "Expected nil error reading response body")
	// Body is empty: the context was cancelled externally, not by our timeout
	// handler, so http.TimeoutHandler writes no timeout message body.
	assert.Equal(t, "", body.String())
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
}

// hijackableRecorder is an httptest.ResponseRecorder that also implements
// http.Hijacker.  It records whether Hijack() was called so we can assert the
// bypass path delivered the original writer to the downstream handler.
type hijackableRecorder struct {
	*httptest.ResponseRecorder
	hijacked bool
}

func (h *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	return nil, nil, nil
}

// TestTimeoutMiddleware_SkipsWebSocketUpgrades verifies that the middleware is
// bypassed for WebSocket upgrade requests.
//
// http.TimeoutHandler wraps the ResponseWriter in a timeoutWriter that does
// NOT implement http.Hijacker, which is required for WebSocket upgrades.  The
// bypass path in our middleware passes the original ResponseWriter directly to
// the handler.  We use a custom ResponseWriter that implements http.Hijacker and
// assert that the handler can successfully type-assert to Hijacker — proving the
// original writer was passed through rather than the timeoutWriter wrapper.
func TestTimeoutMiddleware_SkipsWebSocketUpgrades(t *testing.T) {
	var gotHijacker bool
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, gotHijacker = w.(http.Hijacker)
	})
	handler := TimeoutHandler(50 * time.Millisecond)

	req := httptest.NewRequest("GET", "/searchapi/graphql", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")

	res := &hijackableRecorder{ResponseRecorder: httptest.NewRecorder()}
	handler(nextHandler).ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	require.True(t, gotHijacker,
		"handler should receive a ResponseWriter that implements http.Hijacker when the bypass is active")
}
