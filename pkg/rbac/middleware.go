package rbac

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/database"
	authv1 "k8s.io/api/authentication/v1"
	authov1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

const Threading = true

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
				//creating creates new cache object
				utr := New()
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

				utr.SetTokenTime(clientToken, tokenTime[clientToken]) //set the token and timestamp
				utr.SetUid(uid)                                       //set the uid
				utr.SetExpTime(clientToken, tokenTime[clientToken])   //set an expiration time

				klog.V(5).Info("Current utr data %s", utr)

			} else {
				klog.V(5).Info("User token has not expired and is already validated with token review: %s", utr.data[clientToken])
			}

			// we should re-authenticate after a short period. Maybe 1-2 minutes.

			//2. Now we want to get all the cluster-scoped resources and cache these resources in user struct.

			// //2.check that authenticated users impersonation privilages (authorize):
			// ssar, err := authorize(clientToken, uid, r.Context())

			// if err != nil {
			// 	klog.Warning("Unexpected error while authorizaing the user actions.", err)
			// 	http.Error(w, "{\"message\":\"Unexpected error while authorizing user actions.\"}",
			// 		http.StatusInternalServerError)
			// 	return
			// } else {
			// 	klog.V(5).Info("User authorization successful!")
			// 	klog.V(5).Info("Impersonation for %s successful!", ssar.UID)
			// }

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

//authorize function will return the SelfSubjectAccessReviewSpec{}
func authorize(token string, uid types.UID, ctx context.Context) (*authov1.SelfSubjectAccessReview, error) {

	//first we get the resources then we create te impersonation config

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
	db := database.GetConnection()
	ClusterScopedResources := getClusterScopedResources(db) //returns a map key as group and value as kind : clusterResMap[apigroup] = kind
	fmt.Println(ClusterScopedResources)

	//cache the cluster scoped resources this will be used for all users:

	// namespaceResources, err := listResources()
	// if err != nil {
	// 	klog.Warning("Error getting.", err.Error())
	// }

	for apigroup, kind := range ClusterScopedResources {
		// SelfSubjectAccessReview checks whether or not the current user can perform an action.
		checkSelf := authov1.SelfSubjectAccessReview{ //this will need to be changed to a rules review.
			Spec: authov1.SelfSubjectAccessReviewSpec{
				ResourceAttributes: &authov1.ResourceAttributes{
					Group:    apigroup,
					Verb:     "list",
					Resource: kind,
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
			// data := UserRbac{
			// 	id:         string(uid),
			// 	token:      token,
			// 	namespaces: nil, //WIP
			// 	// Cache Authorization Rules
			// 	namespacedResourceRules: nil, //WIP
			// }
			// fmt.Println(data)
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

// NAMESPACED RESOURCES:
// func listResources() ([]metav1.APIResource, error) {
// 	// allresources := []*metav1.APIGroupList{}
// 	supportedNamepacedResources := []metav1.APIResource{}
// 	// supportedClusterScopedResources := []metav1.APIResource{}//removing since we are getting from cluster scoped from querying database

// 	// get all resources (namespcaed + cluster scopd) on cluster using kubeclient created for user:
// 	apiResources, err := config.KubeClient().ServerPreferredResources()

// 	if err != nil && apiResources == nil { // only return if the list is empty
// 		return nil, err
// 	} else if err != nil {
// 		klog.Warning("ServerPreferredResources could not list all available resources: ", err)
// 	}
// 	for _, apiList := range apiResources {

// 		for _, apiResource := range apiList.APIResources {
// 			for _, verb := range apiResource.Verbs {
// 				if verb == "list" {

// 					//get all resources that have list verb:
// 					if apiResource.Namespaced {
// 						supportedNamepacedResources = append(supportedNamepacedResources, apiResource)
// 					}
// 					// } else {
// 					// 	supportedClusterScopedResources = append(supportedClusterScopedResources, apiResource)
// 					// }
// 				}
// 			}
// 		}
// 	}
// 	return supportedNamepacedResources, err

// }

//function to make call to database to get all cluster-scoped resources:
func getClusterScopedResources(pool *pgxpool.Pool) map[string]string {

	clusterResMap := make(map[string]string)
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)

	query, _, err := ds.Select(goqu.DISTINCT(goqu.L(`"data"->>apigroup`)), goqu.DISTINCT(goqu.L(`"data"->>kind`))).
		Where(goqu.L(`"cluster"::TEXT = "local-cluster"`), goqu.L(`"data"->>namespace = NULL`)).ToSQL()

	if err != nil {
		klog.Errorf("Error creating query [%s]. Error: [%+v]", query, err)
	}

	// items := []map[string]interface{}{}
	rows, err := pool.Query(context.Background(), query, nil...)
	if err != nil {
		klog.Errorf("Error resolving query [%s]. Error: [%+v]", query, err)
	}
	defer rows.Close()

	for rows.Next() {
		var kind string
		var apigroup string
		err = rows.Scan(&kind, &apigroup)
		if err != nil {
			klog.Errorf("Error %s retrieving rows for query:%s", err.Error(), query)
		}
		clusterResMap[apigroup] = kind

	}

	// query, params, err := "SELECT DISTINCT(data->>apigroup, data->>kind) FROM search.resources WHERE cluster='local-cluster' AND namespace=NULL"
	return clusterResMap
}
