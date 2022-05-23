package rbac

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/stolostron/search-v2-api/pkg/config"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

//verifies token (userid) with the TokenReview:
func Middleware(rc *RbacCache) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			var clientToken string
			// var duration_Minute time.Duration = 1 * time.Minute

			//if there is cookie available use that else use the authorization header:
			cookie, err := r.Cookie("acm-access-token-cookie")
			if err == nil {
				clientToken = cookie.Value
				klog.V(5).Info("Got user token from Cookie.")
			} else if r.Header.Get("Authorization") != "" {
				klog.V(5).Info("Got user token from Authorization header.")
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

			if len(rc.Users) > 0 {
				//If there are users, get user:
				ur := rc.GetUser(clientToken, r)
				if ur {
					klog.V(4).Info("Authorization step..")

				}
			} else {

				authenticated, result, err := verifyToken(clientToken, r.Context())
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

				klog.V(5).Info("User authentication successful!")
				t := time.Now()
				klog.V(5).Info("Caching user")
				rc.CacheData(result, t, clientToken)
				klog.V(4).Info("Authorization step..")

			}

			//next we want to authorize
			// klog.V(4).Info("Authorization step..")

			next.ServeHTTP(w, r)
		})
	}
}

func verifyToken(clientId string, ctx context.Context) (bool, *authv1.TokenReview, error) {
	// tokenTime := make(map[string]time.Time)

	tr := authv1.TokenReview{
		Spec: authv1.TokenReviewSpec{
			Token: clientId,
		},
	}
	result, err := config.KubeClient().AuthenticationV1().TokenReviews().Create(ctx, &tr, metav1.CreateOptions{}) //cache tokenreview
	// t := time.Now()
	if err != nil {
		klog.Warning("Error creating the token review.", err.Error())
		// tokenTime[clientId] = t
		return false, nil, err
	}
	klog.V(9).Infof("%v\n", prettyPrint(result.Status))
	if result.Status.Authenticated {
		// tokenTime[clientId] = t
		// uid := result.Status.User.UID

		return true, result, nil
	}
	klog.V(4).Info("User is not authenticated.") //should this be warning or info?
	return false, nil, nil
}

// https://stackoverflow.com/a/51270134
func prettyPrint(i interface{}) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	return string(s)
}
