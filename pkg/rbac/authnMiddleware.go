// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"net/http"
	"strings"

	"k8s.io/klog/v2"
)

type ContextKey string

const ContextAuthTokenKey ContextKey = "authToken"

// isWebSocketHandshake returns true only for a genuine RFC-6455 WebSocket
// opening handshake (GET + Connection:Upgrade + Upgrade:websocket +
// Sec-WebSocket-Key). The middleware skip is safe ONLY for such requests
// because gqlgen's transport.Websocket re-authenticates the connection via
// WebSocketInitFunc. A bare `Upgrade: websocket` header on a POST is NOT a
// handshake and must not bypass authentication.
func isWebSocketHandshake(r *http.Request) bool {
	return r.Method == http.MethodGet &&
		connectionHasUpgradeToken(r.Header.Get("Connection")) &&
		strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		r.Header.Get("Sec-Websocket-Key") != ""
}

// connectionHasUpgradeToken reports whether the Connection header value
// contains an "upgrade" token per RFC 7230 §6.1 (comma-separated list).
func connectionHasUpgradeToken(value string) bool {
	for _, tok := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(tok), "upgrade") {
			return true
		}
	}
	return false
}

// AuthenticateUser verifies token (userid) with the TokenReview:
func AuthenticateUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// Skip authentication middleware ONLY for a genuine WebSocket handshake.
		// gqlgen's transport.Websocket authenticates the connection via
		// WebSocketInitFunc after the upgrade completes.
		if isWebSocketHandshake(r) {
			klog.V(1).Info("Skipping authentication middleware for WebSocket connection.")
			next.ServeHTTP(w, r)
			return
		}

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
			return
		}

		authenticated, err := GetCache().IsValidToken(r.Context(), clientToken)
		if err != nil {
			klog.Warning("Unexpected error while authenticating the request token.", err)
			http.Error(w, "{\"message\":\"Unexpected error while authenticating the request token.\"}",
				http.StatusInternalServerError)
			return

		}
		if !authenticated {
			klog.V(4).Info("Rejecting request: Invalid token.")
			http.Error(w, "{\"message\":\"Invalid token\"}", http.StatusForbidden)
			return
		}

		klog.V(6).Info("User authentication successful!")

		ctx := context.WithValue(r.Context(), ContextAuthTokenKey, clientToken)

		next.ServeHTTP(w, r.WithContext(ctx))

	})
}
