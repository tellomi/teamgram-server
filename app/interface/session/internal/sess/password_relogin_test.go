package sess

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	"github.com/zeromicro/go-zero/core/syncx"
)

func TestLogoutThenPasswordLoginOnSameKey(t *testing.T) {
	const userID int64 = 42
	sess, wrapper := passwordSession(t, mtproto.AuthStateLogout, 0, userID)
	ctx := context.Background()

	if sess.onRpcRequest(ctx, "gate", "127.0.0.1", newInboxMsg(1), &mtproto.TLUsersGetUsers{}, nil) {
		t.Fatal("logged-out key accepted an authed RPC")
	}

	for _, req := range passwordNeededRequests() {
		sess.onRpcResult(ctx, passwordNeededRPC(req))
		if wrapper.state != mtproto.AuthStateNeedPassword || wrapper.AuthUserId != userID {
			t.Fatalf("%T left state %d user %d", req, wrapper.state, wrapper.AuthUserId)
		}
		wrapper.state = mtproto.AuthStateLogout
		wrapper.AuthUserId = 0
	}

	sess.onRpcResult(ctx, passwordNeededRPC(&mtproto.TLAuthSignIn{}))
	if wrapper.state != mtproto.AuthStateNeedPassword || wrapper.AuthUserId != userID {
		t.Fatalf("signIn state %d user %d", wrapper.state, wrapper.AuthUserId)
	}
	if sess.onRpcRequest(ctx, "gate", "127.0.0.1", newInboxMsg(2), &mtproto.TLUsersGetUsers{}, nil) {
		t.Fatal("password step skipped: authed RPC ran before checkPassword")
	}

	sess.onRpcResult(ctx, checkPasswordRPC(userID))
	if wrapper.state != mtproto.AuthStateNormal || wrapper.AuthUserId != userID {
		t.Fatalf("after checkPassword state %d user %d", wrapper.state, wrapper.AuthUserId)
	}
	if !sess.onRpcRequest(ctx, "gate", "127.0.0.1", newInboxMsg(3), &mtproto.TLUsersGetUsers{}, nil) {
		t.Fatal("authed RPC rejected after checkPassword")
	}
}

func TestCheckPasswordFromLogoutBecomesNormal(t *testing.T) {
	const userID int64 = 42
	sess, wrapper := passwordSession(t, mtproto.AuthStateLogout, 0, userID)
	sess.onRpcResult(context.Background(), checkPasswordRPC(userID))
	if wrapper.state != mtproto.AuthStateNormal || wrapper.AuthUserId != userID {
		t.Fatalf("state %d user %d", wrapper.state, wrapper.AuthUserId)
	}
	if !sess.onRpcRequest(context.Background(), "gate", "127.0.0.1", newInboxMsg(4), &mtproto.TLUsersGetUsers{}, nil) {
		t.Fatal("authed RPC rejected")
	}
}

func TestPasswordNeededDoesNotDemoteNormal(t *testing.T) {
	const userID int64 = 42
	sess, wrapper := passwordSession(t, mtproto.AuthStateNormal, userID, 99)
	for _, req := range passwordNeededRequests() {
		sess.onRpcResult(context.Background(), passwordNeededRPC(req))
		if wrapper.state != mtproto.AuthStateNormal || wrapper.AuthUserId != userID {
			t.Fatalf("%T demoted state %d user %d", req, wrapper.state, wrapper.AuthUserId)
		}
	}
	sess.onRpcResult(context.Background(), checkPasswordRPC(99))
	if wrapper.state != mtproto.AuthStateNormal || wrapper.AuthUserId != userID {
		t.Fatalf("late checkPassword changed state %d user %d", wrapper.state, wrapper.AuthUserId)
	}
}

func TestReconnectNeedPasswordBlocksAuthedRPC(t *testing.T) {
	sess, wrapper := passwordSession(t, mtproto.AuthStateUnauthorized, 0, 0)
	wrapper.applyAuthStateData(context.Background(), authsession.MakeTLAuthKeyStateData(&authsession.AuthKeyStateData{
		KeyState: mtproto.AuthStateNeedPassword,
		UserId:   42,
	}).To_AuthKeyStateData())
	if wrapper.state != mtproto.AuthStateNeedPassword || wrapper.AuthUserId != 42 {
		t.Fatalf("reloaded state %d user %d", wrapper.state, wrapper.AuthUserId)
	}
	if sess.onRpcRequest(context.Background(), "gate", "127.0.0.1", newInboxMsg(5), &mtproto.TLUsersGetUsers{}, nil) {
		t.Fatal("reconnecting bound key skipped the password step")
	}
}

func passwordSession(t *testing.T, state int, authUser, boundUser int64) (*session, *MainAuthWrapper) {
	t.Helper()
	const authKeyID int64 = 501
	manager := NewMainAuthWrapperManager(&dao.Dao{
		AuthsessionClient: &fakeAuthsessionClient{userID: boundUser},
	})
	client := authsession.MakeTLClientSession(&authsession.ClientSession{
		AuthKeyId: authKeyID,
		Layer:     232,
	}).To_ClientSession()
	// No Start(): the run loop only logs state, and these cases drive it directly.
	wrapper := &MainAuthWrapper{
		authKeyId:            authKeyID,
		state:                state,
		AuthUserId:           authUser,
		client:               client,
		cb:                   manager,
		closeChan:            make(chan struct{}),
		sessionDataChan:      make(chan interface{}, 1),
		rpcDataChan:          make(chan interface{}, 1),
		running:              syncx.NewAtomicBool(),
		tmpRpcApiMessageList: make([]*rpcApiMessage, 0),
	}
	wrapper.mainAuth = newSessionList(mtproto.AuthKeyTypePerm, wrapper)
	return newSession(1, wrapper.mainAuth), wrapper
}

func passwordNeededRequests() []mtproto.TLObject {
	return []mtproto.TLObject{
		&mtproto.TLAuthSignIn{},
		&mtproto.TLAuthSendCode{},
		&mtproto.TLAuthExportLoginToken{},
		&mtproto.TLAuthImportLoginToken{},
	}
}

func passwordNeededRPC(req mtproto.TLObject) *rpcApiMessage {
	return &rpcApiMessage{
		reqMsg: req,
		rpcResult: &mtproto.TLRpcResult{
			ReqMsgId: 11,
			Result:   mtproto.NewRpcError(mtproto.ErrSessionPasswordNeeded),
		},
	}
}

func checkPasswordRPC(userID int64) *rpcApiMessage {
	user := mtproto.MakeTLUser(&mtproto.User{Id: userID, Self: true}).To_User()
	return &rpcApiMessage{
		reqMsg: &mtproto.TLAuthCheckPassword{},
		rpcResult: &mtproto.TLRpcResult{
			ReqMsgId: 12,
			Result: mtproto.MakeTLAuthAuthorization(&mtproto.Auth_Authorization{
				User: user,
			}).To_Auth_Authorization(),
		},
	}
}
