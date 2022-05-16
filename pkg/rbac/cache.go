package rbac

import "time"

//to get the token and uid
func (tr *UserTokenReviews) GetTimebyToken(token string) (interface{}, bool) {
	if Threading {
		tr.lock.RLock()
		defer tr.lock.RUnlock()

	}
	value, exists := tr.data[token]
	if !exists {
		return value, false
	}
	return value, true

}

func (tr *UserTokenReviews) DoesTokenExist(token string) bool {
	if Threading {
		tr.lock.RLock()
		defer tr.lock.RUnlock()

	}
	_, exists := tr.data[token]

	return exists

}

//to get the token and uid
func (tr *UserTokenReviews) GetTimebyUid(uid string) map[string]time.Time {
	if Threading {
		tr.lock.RLock()
		defer tr.lock.RUnlock()

	}
	if tr.uid == uid {
		return tr.data
	} else {
		return nil
	}

}

// to set the token and uid
func (tr *UserTokenReviews) SetTokenTime(token string, timeCreated time.Time) {
	if Threading {
		tr.lock.Lock()
		defer tr.lock.Unlock()

	}
	tr.data[token] = timeCreated
	var mins time.Duration
	tr.expiresAt = tr.data[token].Add(mins)
}

func (tr *UserTokenReviews) SetExpTime(token string, timeCreated time.Time) {
	if Threading {
		tr.lock.Lock()
		defer tr.lock.Unlock()

	}

	tr.expiresAt = timeCreated.Add(time.Minute * 1) //a minute from creation token
}

// to set the token and uid
func (tr *UserTokenReviews) SetUid(uid string) {
	if Threading {
		tr.lock.Lock()
		defer tr.lock.Unlock()
	}
	tr.uid = uid
}

// this gets the entire cache:
func New() *UserTokenReviews {
	var uid string
	utk := &UserTokenReviews{
		uid:       uid,
		data:      make(map[string]time.Time),
		expiresAt: time.Time{},
	}
	return utk
}

func (tr *UserTokenReviews) Remove(token string) {
	if Threading {
		tr.lock.Lock()
		defer tr.lock.Unlock()
	}
	delete(tr.data, token)
}
