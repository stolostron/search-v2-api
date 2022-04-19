package rbac

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/stolostron/search-v2-api/pkg/config"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

var kClientset *kubernetes.Clientset

func KubeClient() {
	config, err := config.GetClientConfig()
	if err != nil {
		klog.Fatal(err.Error())
	}
	kClientset, err = kubernetes.NewForConfig(config)

	if err != nil {
		klog.Fatal(err.Error())
	}

}

//verifies token (userid) with the TokenReview:
func Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			// Read the value of the client identifier from the request header
			fmt.Fprintf(w, "%s %s %s \n", r.Method, r.URL, r.Proto)

			//if there is cookie in header available use that else use the authorization:
			var tokenId string
			if len(r.Cookies()) != 0 {
				for _, cookie := range r.Cookies() {
					if cookie.Name == "acm-access-token-cookie" {
						log.Println("Cookie: ", cookie.Value)
						tokenId = cookie.Value
					}
				}
			} else {
				if len(r.Header.Get("Authorization")) != 0 {
					log.Println(w, "Authorization is: %s", r.Header.Get("Authorization"))
					tokenId = r.Header.Get("Authorization")
				}
			}
			//Iterate over all header fields
			for k, v := range r.Header {
				fmt.Fprintf(w, "header key %q, value %q\n", k, v)
			}
			//Retrieving and verifying the token:
			// clientId := r.Header.Get("Userid")
			if len(tokenId) == 0 {
				http.Error(w, "Could not find Userid", http.StatusUnauthorized)
				return
			}
			authenticated, err := verifyToken(tokenId, r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if !authenticated {
				http.Error(w, "Invalid token", http.StatusForbidden)
				return
			}
			fmt.Fprintf(w, "Authentication successful!")

		})
	}
}

func verifyToken(clientId string, r *http.Request) (bool, error) {
	ctx := context.Context(r.Context())
	tr := authv1.TokenReview{
		Spec: authv1.TokenReviewSpec{
			Token: clientId,
		},
	}
	result, err := kClientset.AuthenticationV1().TokenReviews().Create(ctx, &tr, metav1.CreateOptions{})
	if err != nil {
		return false, err
	}

	log.Printf("%v\n", prettyPrint(result.Status))
	if result.Status.Authenticated {
		return true, nil
	}
	return false, nil
}

func prettyPrint(i interface{}) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	return string(s)
}
