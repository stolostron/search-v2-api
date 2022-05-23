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

func (rc *RbacCache) GetUser(clientToken string, r *http.Request) *UserRbac {

	// Iterate Users to find if any with a matching token.
	for _, user := range rc.Users {
		if user.DoesTokenExist(clientToken) {
			return user
		} else if user.uid == user.GetIDFromTokenReview(user.UserInfo) {
			return user
		} else {
			return nil
		}
		//user does not exist:	return nil
	}
	return nil

}

func (ur *UserRbac) ValidateToken(clientToken string, r *http.Request) bool {
	// Check the cached timestamp.
	if !time.Now().After(ur.expiresAt) {
		klog.V(5).Info("User token has not expired and is already validated with token review. Can continue with authorization..")
		return true
	} else {
		// if it's been more than a minute re-validate
		klog.V(5).Info("User token authentication over 1 minute, need to re-validate token")
		//re-validate token and if the token is still valid we update the timestamp:
		authenticated, _, _ := verifyToken(clientToken, r.Context())
		if authenticated {
			klog.V(5).Info("User token re-validated. Updating Timestamp.")
			//update timestamp:
			ur.UpdateTokenTime(clientToken)
			//update expiration:
			ur.SetExpTime(clientToken, ur.ValidatedTokens[clientToken])
			// Return true if the token is valid.
			return true
		} else {
			return false
		}
	}
}

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
				if ur != nil {
					//validate user:
					if ur.ValidateToken(clientToken, r) {
						//token is valid can move on to authorization..
						// authorize()
					} else {
						//if authentication is not successful (token no longer valid) remove the token review and reauthenticate:
						ur.Remove(clientToken)

					}
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
				rc.cacheData(result, t, clientToken)

			}

			//next we want to authorize
			klog.V(4).Info("Authorization step..")

			next.ServeHTTP(w, r)
		})
	}
}

func (rc *RbacCache) cacheData(result *authv1.TokenReview, t time.Time, clientToken string) {
	tokenTime := make(map[string]time.Time)
	tokenTime[clientToken] = t
	utr := New(result.Status.User.UID)
	utr.SetUserInfo(result)
	utr.SetTokenTime(clientToken, tokenTime[clientToken])
	utr.SetExpTime(clientToken, tokenTime[clientToken])

	//appendng this user to the RbacCache:
	rc.Users = append(rc.Users, utr)
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
