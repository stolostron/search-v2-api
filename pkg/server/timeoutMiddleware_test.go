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

func TestRequestTimeoutReturnsUnavailableIfTimeout(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(6 * time.Millisecond)
	})
	handler := TimeoutHandler(5 * time.Millisecond)

	req := httptest.NewRequest("POST", "/searchapi/graphql", nil)
	rr := httptest.NewRecorder()

	handler(nextHandler).ServeHTTP(rr, req)
	var body bytes.Buffer
	actual := rr.Result()
	defer actual.Body.Close()
	_, err := io.Copy(&body, actual.Body)

	assert.Nil(t, err, "Expected nil error reading response body")
	assert.Equal(t, body.String(), "Request timed out")
	assert.Equal(t, rr.Code, http.StatusServiceUnavailable)
}

func TestRequestTimeoutReturnsOKIfNoTimeout(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no-op
	})
	handler := TimeoutHandler(5 * time.Millisecond)

	req := httptest.NewRequest("POST", "/searchapi/graphql", nil)
	rr := httptest.NewRecorder()

	handler(nextHandler).ServeHTTP(rr, req)
	var body bytes.Buffer
	actual := rr.Result()
	defer actual.Body.Close()
	_, err := io.Copy(&body, actual.Body)

	assert.Nil(t, err, "Expected nil error reading response body")
	assert.Equal(t, body.String(), "")
	assert.Equal(t, rr.Code, http.StatusOK)
}

func TestRequestTimeoutReturnsUnavailableIfContextTimeout(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no-op
	})
	handler := TimeoutHandler(5 * time.Millisecond)

	req := httptest.NewRequest("POST", "/searchapi/graphql", nil)
	rr := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(context.TODO())
	cancel()
	handler(nextHandler).ServeHTTP(rr, req.WithContext(ctx))

	handler(nextHandler).ServeHTTP(rr, req)
	var body bytes.Buffer
	actual := rr.Result()
	defer actual.Body.Close()
	_, err := io.Copy(&body, actual.Body)

	assert.Nil(t, err, "Expected nil error reading response body")
	assert.Equal(t, body.String(), "") // unavailable because context cancellation, not because timeout handler
	assert.Equal(t, rr.Code, http.StatusServiceUnavailable)
}
