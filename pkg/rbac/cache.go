// Copyright Contributors to the Open Cluster Management projects
package rbac

import (
	"net/http"
	"sync"
	"time"

	authv1 "k8s.io/api/authentication/v1"
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

func (rc *RbacCache) CacheData(result *authv1.TokenReview, t time.Time, clientToken string) {
	tokenTime := make(map[string]time.Time)
	tokenTime[clientToken] = t
	utr := New(result.Status.User.UID)
	utr.SetUserInfo(result)
	utr.SetTokenTime(clientToken, tokenTime[clientToken])
	utr.SetExpTime(clientToken, tokenTime[clientToken])

	//appendng this user to the RbacCache:
	rc.Users = append(rc.Users, utr)
}

func (rc *RbacCache) GetUser(clientToken string, r *http.Request) bool {

	// Iterate Users to find if any with a matching token.
	for _, user := range rc.Users {
		if user.DoesTokenExist(clientToken) {
			//validate user:
			if user.validateToken(clientToken, r) {
				return true
			}
		} else if user.uid == user.GetIDFromTokenReview() {
			//validate user:
			if user.validateToken(clientToken, r) {
				return true
			}
		} else {
			user.removeToken(clientToken)
			return false
		}
		//user does not exist:	return nil
	}
	return false

}

func (ur *UserRbac) validateToken(clientToken string, r *http.Request) bool {
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
