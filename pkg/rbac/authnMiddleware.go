// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/stolostron/search-v2-api/pkg/metric"
	"k8s.io/klog/v2"
)

type ContextKey string

const ContextAuthTokenKey ContextKey = "authToken"

// verifies token (userid) with the TokenReview:
func AuthenticateUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// if there is cookie available use that else use the authorization header:
		var clientToken string
		cookie, err := r.Cookie("acm-access-token-cookie")
		if err == nil {
			clientToken = cookie.Value
			klog.V(6).Info("Got user token from Cookie.")
		} else if r.Header.Get("Authorization") != "" {
			klog.V(6).Info("Got user token from Authorization header.")
			clientToken = r.Header.Get("Authorization")
			// Remove the keyword "Bearer " if it exists in the header.
			clientToken = strings.Replace(clientToken, "Bearer ", "", 1)
		}
		// Retrieving and verifying the token
		if clientToken == "" {
			klog.V(4).Info("Request didn't have a valid authentication token.")
			http.Error(w, "{\"message\":\"Request didn't have a valid authentication token.\"}",
				http.StatusUnauthorized)
			metric.AuthnFailed.WithLabelValues(strconv.Itoa(http.StatusUnauthorized)).Inc()
			return
		}

		authenticated, err := CacheInst.IsValidToken(r.Context(), clientToken)
		if err != nil {
			klog.Warning("Unexpected error while authenticating the request token.", err)
			http.Error(w, "{\"message\":\"Unexpected error while authenticating the request token.\"}",
				http.StatusInternalServerError)
			metric.AuthnFailed.WithLabelValues(strconv.Itoa(http.StatusInternalServerError)).Inc()
			return

		}
		if !authenticated {
			klog.V(4).Info("Rejecting request: Invalid token.")
			http.Error(w, "{\"message\":\"Invalid token\"}", http.StatusForbidden)
			metric.AuthnFailed.WithLabelValues(strconv.Itoa(http.StatusForbidden)).Inc()
			return
		}

		klog.V(6).Info("User authentication successful!")

		ctx := context.WithValue(r.Context(), ContextAuthTokenKey, clientToken)

		next.ServeHTTP(w, r.WithContext(ctx))

	})
}
