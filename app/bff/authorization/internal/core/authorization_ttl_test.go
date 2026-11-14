package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestValidAuthorizationTTLDays(t *testing.T) {
	// Android SessionsActivity and Desktop SelfDestructionBox sessions.
	for _, days := range []int32{7, 30, 90, 180, 183, 365} {
		if !ValidAuthorizationTTLDays(days) {
			t.Fatalf("official %d rejected", days)
		}
	}
	// Already stored. A later save of the same value must not be rejected.
	for _, days := range []int32{182, 548, 730} {
		if !ValidAuthorizationTTLDays(days) {
			t.Fatalf("stored %d rejected", days)
		}
	}
	// 720 is the account self-destruct option, not a session window.
	for _, days := range []int32{0, 1, 6, 8, 14, 31, 60, 91, 184, 366, 720, -1, 1000} {
		if ValidAuthorizationTTLDays(days) {
			t.Fatalf("%d accepted", days)
		}
	}
}

func TestReviewTTLDoesNotFabricateSuccess(t *testing.T) {
	got, err := ttlReply(nil, nil)
	if err == nil || got != nil {
		t.Fatalf("nil result became success %#v %v", got, err)
	}
	storeErr := errors.New("store down")
	if _, err = ttlReply(mtproto.BoolTrue, storeErr); err != storeErr {
		t.Fatalf("store error %v", err)
	}
	got, err = ttlReply(mtproto.BoolTrue, nil)
	if err != nil || got == nil || !mtproto.FromBool(got) {
		t.Fatalf("real result %#v %v", got, err)
	}
}
