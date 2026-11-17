package sess

import (
	"context"
	"time"

	"github.com/teamgram/proto/mtproto"
)

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

// pingRevoked reports that this session must not answer ping.
// bindingRevoked reloads Normal and NeedPassword. After
// updateAccountResetAuthorization the wrapper is already Deleted, and
// that early return used to let onPing send pong anyway.
func (c *session) pingRevoked(ctx context.Context) bool {
	if c == nil || c.sessList == nil || c.sessList.cb == nil {
		return false
	}
	if authStateRevoked(c.sessList.cb.state) {
		return true
	}
	return c.sessList.cb.bindingRevoked(ctx)
}

// authStateRevoked is a binding that must not serve another protected RPC.
// NeedPassword and Normal stay. Logout and Deleted are the reset/logout rows.
func authStateRevoked(state int) bool {
	switch state {
	case mtproto.AuthStateLogout, mtproto.AuthStateDeleted:
		return true
	default:
		return false
	}
}

// replyUnauthorized answers ping and the next RPC with 401
// AUTH_KEY_UNREGISTERED. Official clients leave the chat list for the login
// page on that error. Dropping the packet left the socket open and the app
// still looked signed in. closeDate is pulled forward so the session loop
// stops treating the socket as a live updates channel.
func (c *session) replyUnauthorized(ctx context.Context, gatewayId string, msgID int64) {
	if c == nil {
		return
	}
	c.sendRpcResultToQueue(ctx, gatewayId, msgID, mtproto.NewRpcError(mtproto.ErrAuthKeyUnregistered))
	c.closeDate = time.Now().Unix()
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
