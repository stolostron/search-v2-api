// Copyright Contributors to the Open Cluster Management projects
package rbac

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/stolostron/search-v2-api/pkg/config"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

type RbacCache struct {
	Users []*UserRbac
}

//first we want to cache the authenticated tokenreview response:
type UserRbac struct {
	uid             string
	ValidatedTokens map[string]time.Time //token:timestamp
	UserInfo        *authv1.TokenReview  //response from token review
	expiresAt       time.Time
	lock            sync.RWMutex // use this to avoid runtime error in Go fot simultaneously trying to read and write to a map

	//future fields:
	// In the future we'll add more data like
	// hubClusterScopedRules []Rules
	// rulesByNamespace      map[string]Rules
	// managedClusters       []string // ManagedClusters visible to the user.
}

func (tr *UserRbac) DoesTokenExist(token string) bool {

	tr.lock.RLock()
	defer tr.lock.RUnlock()

	_, exists := tr.ValidatedTokens[token]

	return exists

}

func (tr *UserRbac) SetUserInfo(tokenReviewResponse *authv1.TokenReview) {
	tr.lock.Lock()
	defer tr.lock.Unlock()

	tr.UserInfo = tokenReviewResponse
}

func (tr *UserRbac) GetIDFromTokenReview() string {
	tr.lock.Lock()
	defer tr.lock.Unlock()

	ID := tr.UserInfo.UID

	return string(ID)
}

func (tr *UserRbac) SetTokenTime(token string, timeCreated time.Time) {

	tr.lock.Lock()
	defer tr.lock.Unlock()

	tr.ValidatedTokens[token] = timeCreated
}

func (tr *UserRbac) UpdateTokenTime(token string) {
	tr.lock.Lock()
	defer tr.lock.Unlock()

	timeCreated := time.Now() //updating timestamp if token still valid

	tr.ValidatedTokens[token] = timeCreated
}

func (tr *UserRbac) SetExpTime(token string, timeCreated time.Time) {

	tr.lock.Lock()
	defer tr.lock.Unlock()

	tr.expiresAt = timeCreated.Add(time.Minute * 1) //a minute from token creation
}

// New cache user object:
func New(uid string) *UserRbac {
	utk := &UserRbac{
		uid:             uid,
		ValidatedTokens: make(map[string]time.Time),
		expiresAt:       time.Time{},
	}
	return utk
}

// Removes token from user
func (tr *UserRbac) removeToken(token string) {

	tr.lock.Lock()
	defer tr.lock.Unlock()

	delete(tr.ValidatedTokens, token)
}

func (cache *RbacCache) CacheData(result *authv1.TokenReview, t time.Time, clientToken string) {
	tokenTime := make(map[string]time.Time)
	tokenTime[clientToken] = t
	utr := New(result.Status.User.UID)
	utr.SetUserInfo(result)
	utr.SetTokenTime(clientToken, tokenTime[clientToken])
	utr.SetExpTime(clientToken, tokenTime[clientToken])

	//appendng this user to the RbacCache:
	cache.Users = append(cache.Users, utr)
}

func (cache *RbacCache) ValidateToken(clientId string, r *http.Request, ctx context.Context) (bool, error) {
	// Try to find the user in the cache:
	for _, user := range cache.Users {
		if user.DoesTokenExist(clientId) {
			return user.isValidToken(clientId, r)

		}
	}
	//If not in cache proceed with new token:
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
		time := time.Now()
		cache.CacheData(result, time, clientId) //cache time authenticated,
		return true, nil
	}
	klog.V(4).Info("User is not authenticated.")
	return false, nil
}

func (user *UserRbac) isValidToken(clientToken string, r *http.Request) (bool, error) {
	// Check the cached timestamp
	if !time.Now().After(user.expiresAt) {
		klog.V(5).Info("User token has not expired and is already validated with token review. Can continue with authorization..")
		return true, nil
	} else {
		// if it's been more than a minute re-validate
		klog.V(5).Info("User token authentication over 1 minute, need to re-validate token")
		//re-validate token and if the token is still valid we update the timestamp:
		cache := &RbacCache{}
		authenticated, err := cache.ValidateToken(clientToken, r, r.Context())
		if authenticated {
			klog.V(5).Info("User token re-validated. Updating Timestamp.")
			//update timestamp:
			user.UpdateTokenTime(clientToken)
			//update expiration:
			user.SetExpTime(clientToken, user.ValidatedTokens[clientToken])
			// Return true if the token is valid.
			return true, nil
		} else {
			// cache.removeToken(clientToken)
			return false, err
		}
	}
}
