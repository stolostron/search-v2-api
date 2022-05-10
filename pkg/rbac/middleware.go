package rbac

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/stolostron/search-v2-api/pkg/config"
	authv1 "k8s.io/api/authentication/v1"
	authov1 "k8s.io/api/authorization/v1"

	// machineryV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

//https://github.com/stolostron/search-v2-api/commit/70a43d92cd8a3de0a8a958aff09d238f12433a48
//here we store the authorization rules:
type UserRbac struct {
	// Identify the user
	id    string //get after authenticating user
	token string //the token get from user request do we need?
	// Cache Authorization Rules
	namespaces []string
	// clusterResourceRules    []rule
	namespacedResourceRules map[string]rule
}

type rule struct {
	// Action is always List.
	apigroup string
	kind     string
}

//verifies token (userid) with the TokenReview:
func Middleware() func(http.Handler) http.Handler {
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
			authenticated, uid, err := verifyToken(clientToken, r.Context())
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

			// //2.check that authenticated users impersonation privilages (authorize):
			ssar, err := authorize(clientToken, uid, r.Context())

			if err != nil {
				klog.Warning("Unexpected error while authorizaing the user actions.", err)
				http.Error(w, "{\"message\":\"Unexpected error while authorizing user actions.\"}",
					http.StatusInternalServerError)
				return
			} else {
				klog.V(5).Info("User authorization successful!")
				klog.V(5).Info("Impersonation for %s successful!", ssar.UID)
			}

			// we want to store the ssar
			next.ServeHTTP(w, r)

		})
	}
}

func verifyToken(clientId string, ctx context.Context) (bool, types.UID, error) {
	tr := authv1.TokenReview{
		Spec: authv1.TokenReviewSpec{
			Token: clientId,
		},
	}
	result, err := config.KubeClient().AuthenticationV1().TokenReviews().Create(ctx, &tr, metav1.CreateOptions{})
	if err != nil {
		klog.Warning("Error creating the token review.", err.Error())
		return false, "", err
	}
	klog.V(9).Infof("%v\n", prettyPrint(result.Status))
	if result.Status.Authenticated {
		return true, result.UID, nil
	}
	klog.V(4).Info("User is not authenticated.") //should this be warning or info?
	return false, "", nil
}

//authorize function will return the SelfSubjectAccessReviewSpec{}
func authorize(token string, uid types.UID, ctx context.Context) (*authov1.SelfSubjectAccessReview, error) {

	//create impersonation config that will impersonates user based on UID (from tokereview):
	imConfig := config.GetClientConfig()
	imConfig.Impersonate = rest.ImpersonationConfig{
		UID: string(uid),
	}
	//create a new clientset for the impersonation
	clientset, err := kubernetes.NewForConfig(imConfig)
	if err != nil {
		klog.Warning("Error with creating a new clientset with impersonation config.", err.Error())
	}

	//first we need to get all resource (will need to store this in User struct)
	namespaceResources, _, err := listResources()
	if err != nil {
		klog.Warning("Error getting.", err.Error())
	}

	for _, resources := range namespaceResources {
		// SelfSubjectAccessReview checks whether or not the current user can perform an action.
		checkSelf := authov1.SelfSubjectAccessReview{
			Spec: authov1.SelfSubjectAccessReviewSpec{
				ResourceAttributes: &authov1.ResourceAttributes{
					// Version:  "rbac.authorization.k8s.io/v1",
					Verb:     "list",
					Resource: resources.Name,
				},
			},
		}

		// we check to see if our impersonation config can impersonate the attributes of user:
		result, err := clientset.AuthorizationV1().
			SelfSubjectAccessReviews().
			Create(ctx, &checkSelf, metav1.CreateOptions{})

		if err != nil {
			klog.Warning("Error creating the impersonated access review", err.Error())
		}

		//Status is filled in by the server and indicates whether the request is allowed or not
		if !result.Status.Allowed {
			klog.V(5).Info("Impersonation denied")

		} else {
			klog.V(5).Info("Impersonation allowed")
			data := UserRbac{
				id:         string(uid),
				token:      token,
				namespaces: nil, //WIP
				// Cache Authorization Rules
				namespacedResourceRules: nil, //WIP
			}
			fmt.Println(data)
			return result, err
		}

	}
	return nil, err
}

// https://stackoverflow.com/a/51270134
func prettyPrint(i interface{}) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	return string(s)
}

// list all the resources on cluster
// TODO: will change to querying the database to get resources instead of callin all apis in next iteration)
func listResources() ([]metav1.APIResource, []metav1.APIResource, error) {
	// allresources := []*metav1.APIGroupList{}
	supportedNamepacedResources := []metav1.APIResource{}
	supportedClusterScopedResources := []metav1.APIResource{}

	// get all resources (namespcaed + cluster scopd) on cluster using kubeclient created for user:
	apiResources, err := config.KubeClient().ServerPreferredResources()

	if err != nil && apiResources == nil { // only return if the list is empty
		return nil, nil, err
	} else if err != nil {
		klog.Warning("ServerPreferredResources could not list all available resources: ", err)
	}
	for _, apiList := range apiResources {

		for _, apiResource := range apiList.APIResources {
			for _, verb := range apiResource.Verbs {
				if verb == "list" {

					//get all resources that have list verb:
					if apiResource.Namespaced {
						supportedNamepacedResources = append(supportedNamepacedResources, apiResource)
					} else {
						supportedClusterScopedResources = append(supportedClusterScopedResources, apiResource)
					}
				}
			}
		}
	}
	return supportedNamepacedResources, supportedClusterScopedResources, err

}
