package core

import "testing"

func TestValidAuthorizationTTLDays(t *testing.T) {
	for _, days := range []int32{30, 90, 180, 182, 183, 365, 548, 730} {
		if !ValidAuthorizationTTLDays(days) {
			t.Fatalf("%d rejected", days)
		}
	}
	for _, days := range []int32{0, 1, 7, 31, 60, -1, 1000} {
		if ValidAuthorizationTTLDays(days) {
			t.Fatalf("%d accepted", days)
		}
	}
}
