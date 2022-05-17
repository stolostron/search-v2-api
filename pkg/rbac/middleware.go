package rbac

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/stolostron/search-v2-api/pkg/config"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

//SAVING THIS IN ONLY BRANCH ANX/AUTHENTICATE

const THREADING = true

//first we want to cache the authenticated tokenreview response:
type UserTokenReviews struct {
	uid       string
	data      map[string]time.Time //token:timestamp
	expiresAt time.Time
	lock      sync.RWMutex // use this to avoid runtime error in Go fot simultaneously trying to read and write to a map
}

//verifies token (userid) with the TokenReview:
func Middleware(utr *UserTokenReviews) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			//if there is cookie available use that else use the authorization header:
			var clientToken string
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
			//check if token review already exists and if token has expired
			var duration_Minute time.Duration = 1 * time.Minute
			if !utr.DoesTokenExist(clientToken) || utr.data[clientToken].Sub(utr.expiresAt) < duration_Minute { //if difference between creation and expiration is more than a minute

				authenticated, uid, tokenTime, err := verifyToken(clientToken, r.Context())
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
				//cache token review:
				//creating creates new cache object
				utr := New()

				utr.SetTokenTime(clientToken, tokenTime[clientToken]) //set the token and timestamp
				utr.SetUid(uid)                                       //set the uid
				utr.SetExpTime(clientToken, tokenTime[clientToken])   //set an expiration time

				klog.V(5).Info("Current utr data %s", utr)

			} else {
				klog.V(5).Info("User token has not expired and is already validated with token review: %s", utr.data[clientToken])
			}

			//next we want to authorize
			klog.V(4).Info("token cached")

			// we want to store the ssar
			next.ServeHTTP(w, r)

		})
	}
}

func verifyToken(clientId string, ctx context.Context) (bool, string, map[string]time.Time, error) {
	tokenTime := make(map[string]time.Time)

	tr := authv1.TokenReview{
		Spec: authv1.TokenReviewSpec{
			Token: clientId,
		},
	}
	result, err := config.KubeClient().AuthenticationV1().TokenReviews().Create(ctx, &tr, metav1.CreateOptions{}) //cache tokenreview
	t := time.Now()
	if err != nil {
		klog.Warning("Error creating the token review.", err.Error())
		tokenTime[clientId] = t
		return false, "", nil, err
	}
	klog.V(9).Infof("%v\n", prettyPrint(result.Status))
	if result.Status.Authenticated {
		tokenTime[clientId] = t
		uid := result.Status.User.UID

		return true, uid, tokenTime, nil
	}
	klog.V(4).Info("User is not authenticated.") //should this be warning or info?
	return false, "", nil, nil
}

// https://stackoverflow.com/a/51270134
func prettyPrint(i interface{}) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	return string(s)
}
