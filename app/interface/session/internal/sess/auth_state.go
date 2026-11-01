package sess

import "github.com/teamgram/proto/mtproto"

func shouldRefreshAuthState(state int) bool {
	switch state {
	case mtproto.AuthStateNew,
		mtproto.AuthStateWaitInit,
		mtproto.AuthStateUnauthorized:
		return true
	default:
		return false
	}
}

// authorizationResetTargets reports whether an internal
// updateAccountResetAuthorization should drop this connection.
// Sync delivers that signal to every online session of the user. Only the
// named auth key is logged out. A zero id is ignored so the fanout cannot
// terminate the caller.
func authorizationResetTargets(wrapperAuthKeyID, updateAuthKeyID int64) bool {
	return updateAuthKeyID != 0 && wrapperAuthKeyID == updateAuthKeyID
}

func rpcErrorForAuthState(state int) error {
	switch state {
	case mtproto.AuthStateNew,
		mtproto.AuthStateWaitInit,
		mtproto.AuthStateUnauthorized:
		return mtproto.ErrAuthRestart
	default:
		return mtproto.ErrAuthKeyUnregistered
	}
}
