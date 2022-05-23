package rbac

import (
	"testing"
	"time"
)

func TestBasic(t *testing.T) {
	utr := New("test-uid")
	time := time.Time(time.Now())

	utr.SetTokenTime("sha256~thisisatesttoken", time)

	exists := utr.DoesTokenExist("sha256~thisisatesttoken")
	if !exists {
		t.Error("sha256~thisisatesttoken was not found")
	}

}

// func TestRemove(t *testing.T) {
// 	utr := New("test2-uid")
// 	time := time.Time(time.Now())

// }
