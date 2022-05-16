package rbac

import (
	"testing"
	"time"
)

func TestBasic(t *testing.T) {
	utr := New()
	time := time.Time(time.Now())
	utr.SetTokenTime("sha256~hX8hqMUOjTk1lcL92DXLZwZQvqmIwUPs56ehHzY-qVM", time)
	tim, exists := utr.GetTimebyToken("sha256~hX8hqMUOjTk1lcL92DXLZwZQvqmIwUPs56ehHzY-qVM")
	if !exists {
		t.Error("sha256~hX8hqMUOjTk1lcL92DXLZwZQvqmIwUPs56ehHzY-qVM was not found")
	}
	if tim == nil {
		t.Error("UserTokenReviews[sha256~hX8hqMUOjTk1lcL92DXLZwZQvqmIwUPs56ehHzY-qVM] is nil")
	}
	if tim != time {
		t.Errorf("UserTokenReviews[sha256~hX8hqMUOjTk1lcL92DXLZwZQvqmIwUPs56ehHzY-qVM] != %s", time)
	}
}
