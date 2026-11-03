package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestChangeAuthorizationSettingsFields(t *testing.T) {
	if fields := changeAuthorizationSettingsFields(nil); fields != (authorizationSettingsFields{}) {
		t.Fatalf("nil request = %+v", fields)
	}

	onlyHash := changeAuthorizationSettingsFields(&mtproto.TLAccountChangeAuthorizationSettings{Hash: 42})
	if onlyHash.Confirmed || onlyHash.SetEncrypted || onlyHash.SetCall {
		t.Fatalf("hash only set flags: %+v", onlyHash)
	}

	fields := changeAuthorizationSettingsFields(&mtproto.TLAccountChangeAuthorizationSettings{
		Confirmed:                 true,
		Hash:                      7,
		EncryptedRequestsDisabled: mtproto.BoolFalse,
		CallRequestsDisabled:      mtproto.BoolTrue,
	})
	if !fields.Confirmed || !fields.SetEncrypted || fields.EncryptedRequestsDisabled || !fields.SetCall || !fields.CallRequestsDisabled {
		t.Fatalf("fields = %+v", fields)
	}
}
