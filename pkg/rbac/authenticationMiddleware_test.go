// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// var token string

// func makeFakeToken() string {
// 	t := make([]byte, 4)
// 	rand.Read(t)
// 	return fmt.Sprintf("%x", t)
// }

// // token from cookie provided - should return status okay
func TestTokenCookieAuthenticated(t *testing.T) {

	token := "validtoken"

	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("GET", "https://localhost:4010/playground", nil)

	r.AddCookie(&http.Cookie{Name: "acm-access-token-cookie", Value: token})

	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)

	authen.ServeHTTP(response, r)
	assert.Equal(t, response.Code, http.StatusOK)

}

// token from header provided - should return status okay (but doesn't)
func TestAuthenticateHeaderUser(t *testing.T) {

	token := "validtoken"

	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("GET", "https://localhost:4010/playground", nil)

	r.Header.Add("Authorization", fmt.Sprintf("Bearer %v", token))
	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)

	authen.ServeHTTP(response, r)
	assert.Equal(t, response.Code, http.StatusOK)
}

// Invalid token provided - should return unauthorized
func TestAuthenticateInvalidHeaderUser(t *testing.T) {

	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("GET", "https://localhost:4010/playground", nil)

	r.Header.Add("Authorization", fmt.Sprintf("Bearer %v", 123))
	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)

	authen.ServeHTTP(response, r)
	assert.Equal(t, response.Code, http.StatusUnauthorized)

}

// No token provided - should return unauthorized
func TestAuthenticateNoTokenUser(t *testing.T) {
	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("GET", "https://localhost:4010/playground", nil)

	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)
	authen.ServeHTTP(response, r)
	assert.Equal(t, response.Code, http.StatusUnauthorized)
}

// Empty token provided - should return unauthorized
func TestAuthenticateEmptyTokenUser(t *testing.T) {

	authenticateHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r := httptest.NewRequest("GET", "https://localhost:4010/playground", nil)

	r.Header.Add("Authorization", fmt.Sprintf("Bearer %v", ""))
	response := httptest.NewRecorder()

	authenticateHandler(response, r)
	authen := AuthenticateUser(authenticateHandler)

	authen.ServeHTTP(response, r)
	assert.Equal(t, response.Code, http.StatusUnauthorized)
}
