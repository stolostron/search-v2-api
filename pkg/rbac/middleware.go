package rbac

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/stolostron/search-v2-api/pkg/config"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

//verifies token (userid) with the TokenReview:
func Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			//if there is cookie available use that else use the authorization header:
			var clientToken string
			cookie, err := r.Cookie("acm-access-token-cookie")
			if err == nil {
				clientToken = cookie.Value
				klog.Info("Got user token from Cookie.")
			} else if r.Header.Get("Authorization") != "" {
				klog.Info("Got user token from Authorization header.")
				clientToken = r.Header.Get("Authorization")
			} else {
				http.Error(w, "Could not find a valid token.", http.StatusUnauthorized)
				return
			}
			//Retrieving and verifying the token:
			if clientToken == "" {
				http.Error(w, "Could not find client token", http.StatusUnauthorized)
				return
			}
			authenticated, err := verifyToken(clientToken, r.Context())
			if err != nil {
				http.Error(w, "Unexpected error while authenticating the request", http.StatusInternalServerError)
				return
			}
			if !authenticated {
				http.Error(w, "Invalid token", http.StatusForbidden)
				return
			}
			klog.V(5).Info("User authentication successful!")

			next.ServeHTTP(w, r)

		})

	}
}

func verifyToken(clientId string, ctx context.Context) (bool, error) {
	tr := authv1.TokenReview{
		Spec: authv1.TokenReviewSpec{
			Token: clientId,
		},
	}
	result, err := config.KubeClient().AuthenticationV1().TokenReviews().Create(ctx, &tr, metav1.CreateOptions{})
	if err != nil {
		klog.Warning("Error creating the token review.", err.Error())
		return false, err
	}
	klog.V(9).Infof("%v\n", prettyPrint(result.Status))
	if result.Status.Authenticated {
		return true, nil
	}
	klog.V(5).Info("User is not authenticated.") //should this be warning or info?
	return false, nil
}

// https://stackoverflow.com/a/51270134
func prettyPrint(i interface{}) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	return string(s)
}
