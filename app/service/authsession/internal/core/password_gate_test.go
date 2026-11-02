package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestPasswordPendingStateBlocksBoundLogin(t *testing.T) {
	// Reconnect loaded auth_users (Normal, user 42) before checkPassword.
	state, userID := passwordPendingState(mtproto.AuthStateNormal, 42, 42, true)
	if state != mtproto.AuthStateNeedPassword || userID != 42 {
		t.Fatalf("bound pending = %d user %d", state, userID)
	}

	state, userID = passwordPendingState(mtproto.AuthStateLogout, 0, 42, true)
	if state != mtproto.AuthStateNeedPassword || userID != 42 {
		t.Fatalf("unbound pending = %d user %d", state, userID)
	}

	state, userID = passwordPendingState(mtproto.AuthStateNormal, 42, 0, false)
	if state != mtproto.AuthStateNormal || userID != 42 {
		t.Fatalf("finished login = %d user %d", state, userID)
	}

	state, userID = passwordPendingState(mtproto.AuthStateNormal, 7, 42, true)
	if state != mtproto.AuthStateNormal || userID != 7 {
		t.Fatalf("other user's pending row = %d user %d", state, userID)
	}
}
