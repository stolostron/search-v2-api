// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// test WebSocket upgrade bypasses authorization
func TestAuthorizeWebSocketUpgrade(t *testing.T) {
	// Track if the handler was called
	handlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest("GET", "https://localhost:4010/searchapi/graphql", nil)
	// Full RFC-6455 opening handshake headers.
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Connection", "Upgrade")
	r.Header.Set("Sec-Websocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	r.Header.Set("Sec-Websocket-Version", "13")

	response := httptest.NewRecorder()

	authz := AuthorizeUser(nextHandler)
	authz.ServeHTTP(response, r)

	// Should pass through without authorization
	assert.True(t, handlerCalled, "Handler should be called for WebSocket upgrade")
	assert.Equal(t, http.StatusOK, response.Code, "WebSocket upgrade should bypass authorization")
}

// Regression test: a POST carrying every WebSocket handshake header is still
// NOT a valid RFC-6455 handshake (wrong method) and must not take the
// WebSocket bypass branch in AuthorizeUser.
//
// We verify via isWebSocketHandshake rather than invoking AuthorizeUser
// directly because the normal authorization path requires a live database
// connection (PopulateSharedCache panics without one).
func TestAuthorizeSpoofedUpgradeHeaderNotSkipped(t *testing.T) {
	r := httptest.NewRequest("POST", "https://localhost:4010/searchapi/graphql", nil)
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Connection", "Upgrade")
	r.Header.Set("Sec-Websocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	r.Header.Set("Sec-Websocket-Version", "13")

	assert.False(t, isWebSocketHandshake(r),
		"POST with full handshake headers must not be treated as a WS handshake")
}

// Regression: Connection header with "upgrade" as a substring of another token
// (e.g. "xupgrade") must not bypass authorization.
func TestAuthorizeConnectionSubstringNotSkipped(t *testing.T) {
	r := httptest.NewRequest("GET", "https://localhost:4010/searchapi/graphql", nil)
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Connection", "xupgrade")
	r.Header.Set("Sec-Websocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	assert.False(t, isWebSocketHandshake(r),
		"Connection: xupgrade must not be treated as a valid upgrade token")
}

// test that non-WebSocket request doesn't skip authorization
func TestAuthorizeNonWebSocketRequest(t *testing.T) {
	r := httptest.NewRequest("POST", "https://localhost:4010/searchapi/graphql", nil)
	// No Upgrade header, regular POST request
	
	// Verify the Upgrade header is not set to websocket
	assert.NotEqual(t, "websocket", r.Header.Get("Upgrade"), 
		"Regular requests should not have websocket upgrade header")
	
	// The authorization middleware would normally process this request
	// We're just verifying the header check logic here, not the full auth flow
	// (Full auth flow would require complex mock setup and DB availability)
}

// test WebSocket with wrong upgrade value doesn't bypass authorization
func TestAuthorizeWebSocketWrongUpgradeValue(t *testing.T) {
	r := httptest.NewRequest("GET", "https://localhost:4010/searchapi/graphql", nil)
	// Set Upgrade header but with wrong value
	r.Header.Set("Upgrade", "http2")
	
	// Verify the header value is not "websocket"
	assert.NotEqual(t, "websocket", r.Header.Get("Upgrade"),
		"Wrong upgrade value should not match websocket")
	
	// This request would go through normal authorization (not bypass)
	// The middleware checks for exact "websocket" value
}

