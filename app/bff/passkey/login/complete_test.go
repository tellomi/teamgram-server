package login

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/authorization/loginnotice"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	authsession "github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/enforcement"
	"google.golang.org/grpc/status"
)

type fakeUsers struct {
	user *mtproto.ImmutableUser
}

func (f fakeUsers) UserGetImmutableUser(context.Context, *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	return f.user, nil
}

type fakeSessions struct {
	bound  int64
	device string
}

func (f *fakeSessions) AuthsessionBindAuthKeyUser(_ context.Context, in *authsession.TLAuthsessionBindAuthKeyUser) (*mtproto.Int64, error) {
	f.bound = in.GetUserId()
	return &mtproto.Int64{V: 9}, nil
}

func (f *fakeSessions) AuthsessionGetAuthorization(context.Context, *authsession.TLAuthsessionGetAuthorization) (*mtproto.Authorization, error) {
	return &mtproto.Authorization{DeviceModel: f.device}, nil
}

type fakeUpdates struct {
	n int
}

func (f *fakeUpdates) SyncUpdatesNotMe(context.Context, *syncpb.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	f.n++
	return nil, nil
}

type fakeMsg struct {
	n int
}

func (f *fakeMsg) MsgPushUserMessage(context.Context, *msgpb.TLMsgPushUserMessage) (*mtproto.Bool, error) {
	f.n++
	return mtproto.BoolTrue, nil
}

func testUser(id int64) *mtproto.ImmutableUser {
	return &mtproto.ImmutableUser{User: &mtproto.UserData{Id: id, FirstName: "Ada", Phone: "9996610776"}}
}

func TestAuthorizeBindsAndNotifies(t *testing.T) {
	loginnotice.Reset()
	sessions := &fakeSessions{device: "iPhone 16"}
	updates := &fakeUpdates{}
	msg := &fakeMsg{}
	SetDeps(Deps{Users: fakeUsers{user: testUser(42)}, Sessions: sessions, Updates: updates, Msg: msg})
	t.Cleanup(func() { SetDeps(Deps{}) })

	authz, err := Authorize(context.Background(), &metadata.RpcMetadata{
		PermAuthKeyId: 77,
		Client:        "ios",
		Langpack:      "ios",
		LangCode:      "en",
		ClientAddr:    "203.0.113.8",
	}, 42)
	if err != nil || authz.GetUser().GetId() != 42 {
		t.Fatalf("auth %v %#v", err, authz)
	}
	if sessions.bound != 42 || updates.n != 1 {
		t.Fatalf("bind %d updates %d", sessions.bound, updates.n)
	}
	deadline := time.Now().Add(5 * time.Second)
	for msg.n == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if msg.n != 1 {
		t.Fatalf("login notice %d", msg.n)
	}
}

func TestAuthorizeIdentifierRestricted(t *testing.T) {
	loginnotice.Reset()
	svc := enforcement.New(enforcement.Config{CreateEnabled: true, GrantSecret: []byte("test-grant")})
	if err := svc.RestrictIdentifier(enforcement.Actor{ID: "review", Role: enforcement.RoleAgent}, "phone", "9996610776"); err != nil {
		t.Fatal(err)
	}
	prev := enforcement.Current()
	enforcement.SetCurrent(svc)
	t.Cleanup(func() { enforcement.SetCurrent(prev) })
	sessions := &fakeSessions{}
	SetDeps(Deps{Users: fakeUsers{user: testUser(42)}, Sessions: sessions})
	t.Cleanup(func() { SetDeps(Deps{}) })

	authz, err := Authorize(context.Background(), &metadata.RpcMetadata{PermAuthKeyId: 90}, 42)
	if authz != nil || status.Convert(err).Message() != "IDENTIFIER_RESTRICTED" {
		t.Fatalf("auth %v err %v", authz, err)
	}
	if sessions.bound != 0 {
		t.Fatalf("restricted login bound %d", sessions.bound)
	}
}

func TestAuthorizePasswordNeeded(t *testing.T) {
	loginnotice.Reset()
	prev := password.Swap(password.New(password.Options{Store: passwordStore{has: true}}))
	t.Cleanup(func() { password.Swap(prev) })
	sessions := &fakeSessions{}
	msg := &fakeMsg{}
	SetDeps(Deps{Users: fakeUsers{user: testUser(42)}, Sessions: sessions, Msg: msg})
	t.Cleanup(func() { SetDeps(Deps{}) })

	authz, err := Authorize(context.Background(), &metadata.RpcMetadata{PermAuthKeyId: 88}, 42)
	if authz != nil || status.Convert(err).Message() != "SESSION_PASSWORD_NEEDED" {
		t.Fatalf("auth %v err %v", authz, err)
	}
	if sessions.bound != 42 || msg.n != 0 {
		t.Fatalf("bind %d notice %d", sessions.bound, msg.n)
	}
}

type passwordStore struct {
	has bool
}

func (p passwordStore) Load(context.Context, int64) (password.Record, error) {
	return password.Record{HasPassword: p.has}, nil
}
func (p passwordStore) Save(context.Context, password.Record) error { return nil }
func (p passwordStore) PutChallenge(context.Context, password.Challenge) error {
	return nil
}
func (p passwordStore) TakeChallenge(context.Context, int64) (password.Challenge, bool, error) {
	return password.Challenge{}, false, nil
}
func (p passwordStore) PutLogin(context.Context, int64, int64, int64) error { return nil }
func (p passwordStore) LoginUser(context.Context, int64) (int64, bool, error) {
	return 0, false, nil
}
func (p passwordStore) DeleteLogin(context.Context, int64) error      { return nil }
func (p passwordStore) DeleteUserLogins(context.Context, int64) error { return nil }
