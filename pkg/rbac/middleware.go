package rbac

import (
	"encoding/json"
	"net/http"
	"strings"

	"k8s.io/klog/v2"
)

//verifies token (userid) with the TokenReview:
func AuthenticationMiddleware(cache *RbacCache) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			var clientToken string
			// var duration_Minute time.Duration = 1 * time.Minute

			//if there is cookie available use that else use the authorization header:
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

			isValid, validationError := cache.ValidateToken(clientToken, r, r.Context())
			if validationError != nil {
				klog.Warning("Unexpected error while authenticating the request token.", validationError)
			} else if !isValid {
				klog.V(4).Info("Rejecting request: Invalid token.")
			}

			next.ServeHTTP(w, r)
		})
	}
}

// https://stackoverflow.com/a/51270134
func prettyPrint(i interface{}) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	return string(s)
}
