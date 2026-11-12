package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/statusbook"
)

func TestStatusContactReadFailureDoesNotWritePrivacy(t *testing.T) {
	readErr := errors.New("contact rpc")
	wrote := false
	forgot := false
	err := applyPrivacyChange(mtproto.STATUS_TIMESTAMP, 1, nil, func() ([]int64, []int64, error) {
		return nil, nil, readErr
	}, func() error {
		wrote = true
		return nil
	}, func([]int64) error {
		forgot = true
		return nil
	})
	if !errors.Is(err, readErr) || wrote || forgot {
		t.Fatalf("wrote %v forgot %v err %v", wrote, forgot, err)
	}
}

func TestStatusPrivacyForgetFailureIsReturned(t *testing.T) {
	forgetErr := errors.New("eval")
	wrote := false
	var cleared []int64
	err := applyPrivacyChange(mtproto.STATUS_TIMESTAMP, 1, nil, func() ([]int64, []int64, error) {
		return nil, []int64{2}, nil
	}, func() error {
		wrote = true
		return nil
	}, func(ids []int64) error {
		cleared = ids
		return forgetErr
	})
	if !errors.Is(err, forgetErr) || !wrote {
		t.Fatalf("wrote %v err %v", wrote, err)
	}
	if !containsInt(cleared, 1) || !containsInt(cleared, 2) {
		t.Fatalf("forget ids %v", cleared)
	}
	// The failed clear is retried with the same ids. Bob is in the reverse
	// list for nobody, contacts-only, and a chat-only exception.
	for _, mode := range []string{"nobody", "contacts", "chat exception"} {
		ids := statusbook.StatusPrivacyForgetIDs(1, nil, nil, []int64{2})
		if mode == "contacts" {
			ids = statusbook.StatusPrivacyForgetIDs(1, nil, []int64{2}, nil)
		}
		if !containsInt(ids, 2) {
			t.Fatalf("%s ids %v", mode, ids)
		}
	}
}

func containsInt(ids []int64, want int64) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
