package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/account/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/account/internal/svc"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	authsession "github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	authsessionclient "github.com/teamgram/teamgram-server/app/service/authsession/client"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/push"

	"github.com/zeromicro/go-zero/core/logx"
)

func TestAccountResetAuthorizationUnregistersThatSession(t *testing.T) {
	ctx := context.Background()
	store := &push.MemoryStore{}
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 100, UserID: 9, TokenType: push.TokenAPNS, Token: "current"})
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 200, UserID: 9, TokenType: push.TokenAPNS, Token: "terminated"})
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 200, UserID: 9, TokenType: push.TokenFCM, Token: "terminated-fcm"})

	c := accountCore(ctx, store, &accountAuthStub{resetIDs: []int64{200, 0, 100}})
	ok, err := c.AccountResetAuthorization(&mtproto.TLAccountResetAuthorization{Hash: 55})
	if err != nil || ok != mtproto.BoolTrue {
		t.Fatalf("%v %v", ok, err)
	}
	left, _ := store.ListByUser(ctx, 9)
	if len(left) != 1 || left[0].Token != "current" {
		t.Fatalf("after reset %+v", left)
	}
}

func TestAccountResetAuthorizationKeepsDevicesWhenResetFails(t *testing.T) {
	ctx := context.Background()
	store := &push.MemoryStore{}
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 200, UserID: 9, TokenType: push.TokenAPNS, Token: "terminated"})
	c := accountCore(ctx, store, &accountAuthStub{resetErr: errors.New("fresh")})
	if _, err := c.AccountResetAuthorization(&mtproto.TLAccountResetAuthorization{Hash: 55}); err == nil {
		t.Fatal("expected reset error")
	}
	left, _ := store.ListByUser(ctx, 9)
	if len(left) != 1 {
		t.Fatalf("devices dropped on failed reset: %+v", left)
	}
}

func TestPurgeAccountUnregistersEveryDevice(t *testing.T) {
	ctx := context.Background()
	store := &push.MemoryStore{}
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 100, UserID: 9, TokenType: push.TokenAPNS, Token: "phone"})
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 200, UserID: 9, TokenType: push.TokenFCM, Token: "tablet"})
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 300, UserID: 4, TokenType: push.TokenAPNS, Token: "someone-else"})

	c := accountCore(ctx, store, &accountAuthStub{resetIDs: []int64{100, 200}})
	c.svcCtx.Dao.UserClient = userStub{}
	ok, err := c.purgeAccount(&mtproto.UserData{Phone: "8613800138000"}, "test")
	if err != nil || ok != mtproto.BoolTrue {
		t.Fatalf("%v %v", ok, err)
	}
	left, _ := store.ListByUser(ctx, 9)
	if len(left) != 0 {
		t.Fatalf("deleted account still has %+v", left)
	}
	other, _ := store.ListByUser(ctx, 4)
	if len(other) != 1 || other[0].Token != "someone-else" {
		t.Fatalf("other user %+v", other)
	}
	ok, err = c.purgeAccount(&mtproto.UserData{Phone: "8613800138000"}, "test")
	if err != nil || ok != mtproto.BoolTrue {
		t.Fatal(err)
	}
}

type accountAuthStub struct {
	authsessionclient.AuthsessionClient
	resetIDs []int64
	resetErr error
}

func (s *accountAuthStub) AuthsessionResetAuthorization(context.Context, *authsession.TLAuthsessionResetAuthorization) (*authsession.Vector_Long, error) {
	if s.resetErr != nil {
		return nil, s.resetErr
	}
	return &authsession.Vector_Long{Datas: s.resetIDs}, nil
}

func (s *accountAuthStub) AuthsessionUnbindAuthKeyUser(context.Context, *authsession.TLAuthsessionUnbindAuthKeyUser) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}

type accountSyncStub struct {
	syncclient.SyncClient
}

func (accountSyncStub) SyncUpdatesMe(context.Context, *sync.TLSyncUpdatesMe) (*mtproto.Void, error) {
	return nil, nil
}

type userStub struct {
	userclient.UserClient
}

func (userStub) UserDeleteUser(context.Context, *user.TLUserDeleteUser) (*mtproto.Bool, error) {
	return mtproto.BoolTrue, nil
}

func accountCore(ctx context.Context, store push.Store, auth authsessionclient.AuthsessionClient) *AccountCore {
	return &AccountCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{
			Dao: &dao.Dao{
				AuthsessionClient: auth,
				SyncClient:        accountSyncStub{},
			},
			Devices: store,
		},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 9, PermAuthKeyId: 100, AuthId: 100},
	}
}
