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
	Instcache = newMockCache()
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
