// Copyright Contributors to the Open Cluster Management projects
package rbac

import (
	"sync"
	"time"

	authv1 "k8s.io/api/authentication/v1"
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

//to get the token and uid
func (tr *UserRbac) GetTimebyToken(token string) (interface{}, bool) {

	tr.lock.RLock()
	defer tr.lock.RUnlock()

	value, exists := tr.ValidatedTokens[token]

	return value, exists

}

// check existence of user with token:
func (tr *UserRbac) DoesTokenExist(token string) bool {

	tr.lock.RLock()
	defer tr.lock.RUnlock()

	_, exists := tr.ValidatedTokens[token]

	return exists

}

// func (rc *RbacCache) DoesUserExist(token string) bool {
// 	var exists bool
// 	for _, tr := range rc.Users {

// 		_, exists = tr.ValidatedTokens[token]
// 	}
// 	return exists

// }

//to get the token and uid
func (tr *UserRbac) GetTimebyUid(uid string) map[string]time.Time {

	tr.lock.RLock()
	defer tr.lock.RUnlock()

	if tr.uid == uid {
		return tr.ValidatedTokens
	} else {
		return nil
	}

}

func (tr *UserRbac) SetUserInfo(tokenReviewResponse *authv1.TokenReview) {
	tr.lock.Lock()
	defer tr.lock.Unlock()

	tr.UserInfo = tokenReviewResponse
}

func (tr *UserRbac) GetIDFromTokenReview(tokenReviewResponse *authv1.TokenReview) string {
	tr.lock.Lock()
	defer tr.lock.Unlock()

	ID := tr.UserInfo.UID

	return string(ID)
}

// to set the token and uid
func (tr *UserRbac) SetTokenTime(token string, timeCreated time.Time) {

	tr.lock.Lock()
	defer tr.lock.Unlock()

	tr.ValidatedTokens[token] = timeCreated
	// var mins time.Duration
	// tr.expiresAt = tr.ValidatedTokens[token].Add(mins)
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

// this gets the entire cache:
func New(uid string) *UserRbac {
	utk := &UserRbac{
		uid:             uid,
		ValidatedTokens: make(map[string]time.Time),
		expiresAt:       time.Time{},
	}
	return utk
}

//this we will use if the token is old
func (tr *UserRbac) Remove(token string) {

	tr.lock.Lock()
	defer tr.lock.Unlock()

	delete(tr.ValidatedTokens, token)
}
