// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Runs before the tests
func TestMain(m *testing.M) {
	// Replace the cache with a mock cache with a fake kubernetes client.
	cacheInst = newMockCache()
	code := m.Run()
	os.Exit(code)
}

//test token from cookie
func TestTokenCookieAuthenticated(t *testing.T) {
	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("POST", "https://localhost:4010/searchapi/graphql", nil)

	r.AddCookie(&http.Cookie{Name: "acm-access-token-cookie", Value: "mytesttoken"})

	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)

	authen.ServeHTTP(response, r)
	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.Equal(t, "{\"message\":\"Invalid token\"}\n", response.Body.String())

}

//test invalid cookie name
func TestTokenInvalidCookieAuthenticated(t *testing.T) {

	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("POST", "https://localhost:4010/searchapi/graphql", nil)

	r.AddCookie(&http.Cookie{Name: "acm-token", Value: "mytesttoken"})

	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)

	authen.ServeHTTP(response, r)
	assert.Equal(t, http.StatusUnauthorized, response.Code) //token is not provided/invalid
	assert.Equal(t, "{\"message\":\"Request didn't have a valid authentication token.\"}\n", response.Body.String())
}

//test invalid cookie value
func TestTokenInvalidCookieValueAuthenticated(t *testing.T) {

	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("POST", "https://localhost:4010/searchapi/graphql", nil)

	r.AddCookie(&http.Cookie{Name: "acm-access-token-cookie", Value: ""})

	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)

	authen.ServeHTTP(response, r)
	assert.Equal(t, http.StatusUnauthorized, response.Code) //token is not provided/invalid
	assert.Equal(t, "{\"message\":\"Request didn't have a valid authentication token.\"}\n", response.Body.String())
}

// test Authorization header bearer token
func TestAuthenticateHeaderUser(t *testing.T) {

	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("POST", "https://localhost:4010/searchapi/graphql", nil)

	r.Header.Add("Authorization", fmt.Sprintf("Bearer %v", "mytesttoken"))
	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)

	authen.ServeHTTP(response, r)

	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.Equal(t, "{\"message\":\"Invalid token\"}\n", response.Body.String())
}

//test invalid header key
func TestAuthenticateInvalidHeaderUser(t *testing.T) {

	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("POST", "https://localhost:4010/searchapi/graphql", nil)

	r.Header.Add("Client-ID", "mytesttoken")
	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)

	authen.ServeHTTP(response, r)
	assert.Equal(t, http.StatusUnauthorized, response.Code) //token is not provided/invalid
	assert.Equal(t, "{\"message\":\"Request didn't have a valid authentication token.\"}\n", response.Body.String())

}

//test no token provided
func TestAuthenticateNoTokenUser(t *testing.T) {
	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("POST", "https://localhost:4010/searchapi/graphql", nil)

	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)
	authen.ServeHTTP(response, r)
	assert.Equal(t, http.StatusUnauthorized, response.Code) //token is not provided/invalid
	assert.Equal(t, "{\"message\":\"Request didn't have a valid authentication token.\"}\n", response.Body.String())
}

// test empty header token value
func TestAuthenticateEmptyTokenUser(t *testing.T) {

	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("POST", "https://localhost:4010/searchapi/graphql", nil)

	r.Header.Add("Authorization", fmt.Sprintf("Bearer %v", ""))
	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)

	authen.ServeHTTP(response, r)
	assert.Equal(t, http.StatusUnauthorized, response.Code) //token is not provided/invalid
	assert.Equal(t, "{\"message\":\"Request didn't have a valid authentication token.\"}\n", response.Body.String())

}

// test that a genuine WebSocket handshake bypasses the HTTP auth middleware
// (auth is enforced post-upgrade by WebSocketInitFunc).
func TestAuthenticateWebSocketUpgrade(t *testing.T) {
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
	// No authentication token provided

	response := httptest.NewRecorder()

	authen := AuthenticateUser(nextHandler)
	authen.ServeHTTP(response, r)

	// Should pass through without authentication
	assert.True(t, handlerCalled, "Handler should be called for WebSocket upgrade")
	assert.Equal(t, http.StatusOK, response.Code, "WebSocket upgrade should bypass authentication")
}

// Regression test: a POST request that merely sets Upgrade: websocket is NOT a
// WebSocket handshake and MUST NOT bypass authentication. This is the attack
// vector against /federated.
func TestAuthenticateSpoofedUpgradeHeaderRejected(t *testing.T) {
	handlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	})

	r := httptest.NewRequest("POST", "https://localhost:4010/federated", nil)
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Connection", "Upgrade")
	r.Header.Set("Sec-Websocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	r.Header.Set("Sec-Websocket-Version", "13")
	// No authentication token provided

	response := httptest.NewRecorder()

	authen := AuthenticateUser(nextHandler)
	authen.ServeHTTP(response, r)

	assert.False(t, handlerCalled, "Handler must not be called for spoofed Upgrade header on POST")
	assert.Equal(t, http.StatusUnauthorized, response.Code)
}

// A bare Upgrade header without Connection:Upgrade and Sec-Websocket-Key is
// not a valid handshake and must not bypass authentication.
func TestAuthenticateIncompleteWebSocketHandshakeRejected(t *testing.T) {
	handlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	})

	r := httptest.NewRequest("GET", "https://localhost:4010/searchapi/graphql", nil)
	r.Header.Set("Upgrade", "websocket") // missing Connection + Sec-Websocket-Key

	response := httptest.NewRecorder()

	authen := AuthenticateUser(nextHandler)
	authen.ServeHTTP(response, r)

	assert.False(t, handlerCalled, "Handler must not be called for incomplete WS handshake")
	assert.Equal(t, http.StatusUnauthorized, response.Code)
}

// Regression: Connection header containing "upgrade" as a substring of another
// token (e.g. "xupgrade") must NOT be treated as a valid handshake.
func TestAuthenticateConnectionSubstringRejected(t *testing.T) {
	handlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	})

	r := httptest.NewRequest("GET", "https://localhost:4010/searchapi/graphql", nil)
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Connection", "xupgrade")
	r.Header.Set("Sec-Websocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	response := httptest.NewRecorder()

	authen := AuthenticateUser(nextHandler)
	authen.ServeHTTP(response, r)

	assert.False(t, handlerCalled, "Handler must not be called when Connection token is a non-upgrade substring")
	assert.Equal(t, http.StatusUnauthorized, response.Code)
}

// test non-WebSocket request still requires authentication
func TestAuthenticateNonWebSocketRequiresAuth(t *testing.T) {
	handlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	})

	r := httptest.NewRequest("POST", "https://localhost:4010/searchapi/graphql", nil)
	// No Upgrade header, regular POST request
	// No authentication token provided

	response := httptest.NewRecorder()

	authen := AuthenticateUser(nextHandler)
	authen.ServeHTTP(response, r)

	// Should NOT pass through, authentication required
	assert.False(t, handlerCalled, "Handler should not be called without authentication")
	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.Equal(t, "{\"message\":\"Request didn't have a valid authentication token.\"}\n", response.Body.String())
}
