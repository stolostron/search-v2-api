// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

//test valid token from cookie
func TestTokenCookieAuthenticated(t *testing.T) {
	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("POST", "https://localhost:4010/searchapi/graphql", nil)

	r.AddCookie(&http.Cookie{Name: "acm-access-token-cookie", Value: "mytesttoken"})

	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)

	authen.ServeHTTP(response, r)
	assert.Equal(t, response.Code, http.StatusInternalServerError)
	assert.Equal(t, response.Body.String(), "{\"message\":\"Unexpected error while authenticating the request token.\"}\n")

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
	assert.Equal(t, response.Code, http.StatusUnauthorized) //token is not provided/invalid
	assert.Equal(t, response.Body.String(), "{\"message\":\"Request didn't have a valid authentication token.\"}\n")
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
	assert.Equal(t, response.Code, http.StatusUnauthorized) //token is not provided/invalid
	assert.Equal(t, response.Body.String(), "{\"message\":\"Request didn't have a valid authentication token.\"}\n")
}

// test valid header bearer token
func TestAuthenticateHeaderUser(t *testing.T) {

	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("POST", "https://localhost:4010/searchapi/graphql", nil)

	r.Header.Add("Authorization", fmt.Sprintf("Bearer %v", "mytesttoken"))
	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)

	authen.ServeHTTP(response, r)
	assert.Equal(t, response.Code, http.StatusInternalServerError)
	assert.Equal(t, response.Body.String(), "{\"message\":\"Unexpected error while authenticating the request token.\"}\n")
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
	assert.Equal(t, response.Code, http.StatusUnauthorized) //token is not provided/invalid
	assert.Equal(t, response.Body.String(), "{\"message\":\"Request didn't have a valid authentication token.\"}\n")

}

//test no token provided
func TestAuthenticateNoTokenUser(t *testing.T) {
	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("POST", "https://localhost:4010/searchapi/graphql", nil)

	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)
	authen.ServeHTTP(response, r)
	assert.Equal(t, response.Code, http.StatusUnauthorized) //token is not provided/invalid
	assert.Equal(t, response.Body.String(), "{\"message\":\"Request didn't have a valid authentication token.\"}\n")
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
	assert.Equal(t, response.Code, http.StatusUnauthorized) //token is not provided/invalid
	assert.Equal(t, response.Body.String(), "{\"message\":\"Request didn't have a valid authentication token.\"}\n")

}
