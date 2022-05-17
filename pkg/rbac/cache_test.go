package rbac

import (
	"testing"
	"time"
)

func TestBasic(t *testing.T) {
	utr := New()
	time := time.Time(time.Now())
	utr.SetTokenTime("sha256~thisisatesttoken", time)
	tim, exists := utr.GetTimebyToken("sha256~thisisatesttoken")
	if !exists {
		t.Error("sha256~thisisatesttoken was not found")
	}
	if tim == nil {
		t.Error("UserTokenReviews[sha256~thisisatesttoken] is nil")
	}
	if tim != time {
		t.Errorf("UserTokenReviews[sha256~thisisatesttoken] != %s", time)
	}
}
