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

	token := "validtoken"
	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("GET", "https://localhost:4010/playground", nil)

	r.AddCookie(&http.Cookie{Name: "acm-access-token-cookie", Value: token})

	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)

	authen.ServeHTTP(response, r)
	assert.Equal(t, response.Code, http.StatusForbidden) //token is provided but not authenticated
}

//test invalid cookie name
func TestTokenInvalidCookieAuthenticated(t *testing.T) {

	token := "validtoken"
	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("GET", "https://localhost:4010/playground", nil)

	r.AddCookie(&http.Cookie{Name: "acm-token", Value: token})

	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)

	authen.ServeHTTP(response, r)
	assert.Equal(t, response.Code, http.StatusUnauthorized) //token is not provided/invalid
}

//test invalid cookie value
func TestTokenInvalidCookieValueAuthenticated(t *testing.T) {

	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("GET", "https://localhost:4010/playground", nil)

	r.AddCookie(&http.Cookie{Name: "acm-access-token-cookie", Value: ""})

	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)

	authen.ServeHTTP(response, r)
	assert.Equal(t, response.Code, http.StatusUnauthorized) //token is not provided/invalid
}

// test valid header bearer token
func TestAuthenticateHeaderUser(t *testing.T) {

	token := "validtoken"

	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("GET", "https://localhost:4010/playground", nil)

	r.Header.Add("Authorization", fmt.Sprintf("Bearer %v", token))
	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)

	authen.ServeHTTP(response, r)
	assert.Equal(t, response.Code, http.StatusForbidden) //token is provided but not authenticated
}

//test invalid header key
func TestAuthenticateInvalidHeaderUser(t *testing.T) {
	token := "validtoken"

	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("GET", "https://localhost:4010/playground", nil)

	r.Header.Add("Client-ID", token)
	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)

	authen.ServeHTTP(response, r)
	assert.Equal(t, response.Code, http.StatusUnauthorized) //token is not provided/invalid

}

//test no token provided
func TestAuthenticateNoTokenUser(t *testing.T) {
	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("GET", "https://localhost:4010/playground", nil)

	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)
	authen.ServeHTTP(response, r)
	assert.Equal(t, response.Code, http.StatusUnauthorized) //token is not provided/invalid
}

// test empty header token value
func TestAuthenticateEmptyTokenUser(t *testing.T) {

	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("GET", "https://localhost:4010/playground", nil)

	r.Header.Add("Authorization", fmt.Sprintf("Bearer %v", ""))
	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)

	authen.ServeHTTP(response, r)
	assert.Equal(t, response.Code, http.StatusUnauthorized) //token is not provided/invalid

}
