// Copyright Contributors to the Open Cluster Management projects
package rbac

import "time"

//to get the token and uid
func (tr *UserRbac) GetTimebyToken(token string) (interface{}, bool) {

	tr.lock.RLock()
	defer tr.lock.RUnlock()

	value, exists := tr.ValidatedTokens[token]

	return value, exists

}

func (tr *UserRbac) DoesTokenExist(token string) bool {

	tr.lock.RLock()
	defer tr.lock.RUnlock()

	_, exists := tr.ValidatedTokens[token]

	return exists

}

// func (rb *RbacCache) DoesUserExist(token string) bool {

// 	 _, exists := rb[]

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

func (tr *UserRbac) SetUserInfo(tokenReviewResponse interface{}) {
	tr.lock.Lock()
	defer tr.lock.Unlock()

	tr.UserInfo = tokenReviewResponse
}

// to set the token and uid
func (tr *UserRbac) SetTokenTime(token string, timeCreated time.Time) {

	tr.lock.Lock()
	defer tr.lock.Unlock()

	tr.ValidatedTokens[token] = timeCreated
	// var mins time.Duration
	// tr.expiresAt = tr.ValidatedTokens[token].Add(mins)
}

func (tr *UserRbac) SetExpTime(token string, timeCreated time.Time) {

	tr.lock.Lock()
	defer tr.lock.Unlock()

	tr.expiresAt = timeCreated.Add(time.Minute * 1) //a minute from token creation
}

// to set the token and uid
// func (tr *UserRbac) SetUid(uid string) {

// 	tr.lock.Lock()
// 	defer tr.lock.Unlock()

// 	tr.uid = uid
// }

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
