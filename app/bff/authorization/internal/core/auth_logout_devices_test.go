package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/svc"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	authsession "github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	authsessionclient "github.com/teamgram/teamgram-server/app/service/authsession/client"
	"github.com/teamgram/teamgram-server/pkg/push"

	"github.com/zeromicro/go-zero/core/logx"
)

func TestLogoutDeviceKeys(t *testing.T) {
	if got := logoutDeviceKeys(0, 7); len(got) != 1 || got[0] != 0 {
		t.Fatalf("perm 0 = %v", got)
	}
	got := logoutDeviceKeys(10, 10)
	if len(got) != 1 || got[0] != 10 {
		t.Fatalf("same key %v", got)
	}
	got = logoutDeviceKeys(10, 11)
	if len(got) != 2 || got[0] != 10 || got[1] != 11 {
		t.Fatalf("perm and temp %v", got)
	}
}

func TestAuthLogOutUnregistersThatSession(t *testing.T) {
	ctx := context.Background()
	store := &push.MemoryStore{}
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 100, UserID: 9, TokenType: push.TokenAPNS, Token: "this-phone"})
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 100, UserID: 9, TokenType: push.TokenAPNSVoIP, Token: "this-voip"})
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 200, UserID: 9, TokenType: push.TokenAPNS, Token: "other-phone"})

	c := authCore(ctx, store, &authStub{})
	out, err := c.AuthLogOut(&mtproto.TLAuthLogOut{})
	if err != nil || out == nil || len(out.FutureAuthToken) == 0 {
		t.Fatalf("logout %v %v", out, err)
	}
	left, err := store.ListByUser(ctx, 9)
	if err != nil || len(left) != 1 || left[0].Token != "other-phone" {
		t.Fatalf("after logout %+v %v", left, err)
	}

	out, err = c.AuthLogOut(&mtproto.TLAuthLogOut{})
	if err != nil || out == nil {
		t.Fatal(err)
	}
	left, _ = store.ListByUser(ctx, 9)
	if len(left) != 1 {
		t.Fatalf("second logout %+v", left)
	}
}

func TestAuthLogOutKeepsDevicesWhenUnbindFails(t *testing.T) {
	ctx := context.Background()
	store := &push.MemoryStore{}
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 100, UserID: 9, TokenType: push.TokenAPNS, Token: "this-phone"})
	c := authCore(ctx, store, &authStub{unbindErr: errors.New("unbind")})
	if _, err := c.AuthLogOut(&mtproto.TLAuthLogOut{}); err == nil {
		t.Fatal("expected unbind error")
	}
	left, _ := store.ListByUser(ctx, 9)
	if len(left) != 1 {
		t.Fatalf("devices dropped on failed logout: %+v", left)
	}
}

func TestAuthResetAuthorizationsUnregistersOtherSessions(t *testing.T) {
	ctx := context.Background()
	store := &push.MemoryStore{}
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 100, UserID: 9, TokenType: push.TokenAPNS, Token: "current"})
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 200, UserID: 9, TokenType: push.TokenAPNS, Token: "other"})
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 201, UserID: 9, TokenType: push.TokenAPNSVoIP, Token: "other-voip"})

	c := authCore(ctx, store, &authStub{resetIDs: []int64{200, 201, 100, 0}})
	ok, err := c.AuthResetAuthorizations(&mtproto.TLAuthResetAuthorizations{})
	if err != nil || ok != mtproto.BoolTrue {
		t.Fatalf("%v %v", ok, err)
	}
	left, _ := store.ListByUser(ctx, 9)
	if len(left) != 1 || left[0].Token != "current" {
		t.Fatalf("after reset-all %+v", left)
	}
}

func TestAuthResetAuthorizationsKeepsDevicesWhenResetFails(t *testing.T) {
	ctx := context.Background()
	store := &push.MemoryStore{}
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 200, UserID: 9, TokenType: push.TokenAPNS, Token: "other"})
	c := authCore(ctx, store, &authStub{resetErr: errors.New("fresh")})
	if _, err := c.AuthResetAuthorizations(&mtproto.TLAuthResetAuthorizations{}); err == nil {
		t.Fatal("expected reset error")
	}
	left, _ := store.ListByUser(ctx, 9)
	if len(left) != 1 {
		t.Fatalf("devices dropped on failed reset: %+v", left)
	}
}

func authCore(ctx context.Context, store push.Store, auth authsessionclient.AuthsessionClient) *AuthorizationCore {
	return &AuthorizationCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{
			Dao: &dao.Dao{
				AuthsessionClient: auth,
				SyncClient:        syncStub{},
			},
			Devices: store,
		},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 9, PermAuthKeyId: 100, AuthId: 100},
	}
}

type authStub struct {
	authsessionclient.AuthsessionClient
	resetIDs  []int64
	resetErr  error
	unbindErr error
}

func (s *authStub) AuthsessionResetAuthorization(context.Context, *authsession.TLAuthsessionResetAuthorization) (*authsession.Vector_Long, error) {
	if s.resetErr != nil {
		return nil, s.resetErr
	}
	return &authsession.Vector_Long{Datas: s.resetIDs}, nil
}

func (s *authStub) AuthsessionUnbindAuthKeyUser(context.Context, *authsession.TLAuthsessionUnbindAuthKeyUser) (*mtproto.Bool, error) {
	if s.unbindErr != nil {
		return nil, s.unbindErr
	}
	return mtproto.BoolTrue, nil
}

type syncStub struct {
	syncclient.SyncClient
}

func (syncStub) SyncUpdatesMe(context.Context, *sync.TLSyncUpdatesMe) (*mtproto.Void, error) {
	return nil, nil
}
