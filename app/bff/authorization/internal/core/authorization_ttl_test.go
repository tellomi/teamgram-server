package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

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
