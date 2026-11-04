package core

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpcmeta "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/usernames/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/usernames/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// tellomiResolver is the user service after migrate-20261107-tellomi-ai-name.sql:
// @tellomi is user 777001, display name Tellomi AI.
type tellomiResolver struct {
	userclient.UserClient
}

func (tellomiResolver) UserResolveUsername(_ context.Context, in *userpb.TLUserResolveUsername) (*mtproto.Peer, error) {
	if in.GetUsername() != "tellomi" {
		return nil, mtproto.ErrUsernameNotOccupied
	}
	return mtproto.MakePeerUser(777001), nil
}

func (tellomiResolver) UserGetMutableUsers(_ context.Context, in *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	users := make([]*mtproto.ImmutableUser, 0, len(in.GetId()))
	for _, id := range in.GetId() {
		first, name := "Viewer", ""
		if id == 777001 {
			first, name = "Tellomi AI", "tellomi"
		}
		users = append(users, mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
			User: mtproto.MakeTLUserData(&mtproto.UserData{
				Id:        id,
				FirstName: first,
				Username:  name,
				UserType:  userpb.UserTypeRegular,
			}).To_UserData(),
		}).To_ImmutableUser())
	}
	return &userpb.Vector_ImmutableUser{Datas: users}, nil
}

func TestContactsResolveUsernameTellomi(t *testing.T) {
	const viewer int64 = 42
	buf, err := proto.Marshal(&rpcmeta.RpcMetadata{UserId: viewer, PermAuthKeyId: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"rpc_metadata", base64.StdEncoding.EncodeToString(buf),
	))
	c := New(ctx, &svc.ServiceContext{Dao: &dao.Dao{UserClient: tellomiResolver{}}})

	got, err := c.ContactsResolveUsername(&mtproto.TLContactsResolveUsername{Username: "tellomi"})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetPeer().GetUserId() != 777001 {
		t.Fatalf("peer = %d", got.GetPeer().GetUserId())
	}
	var ai *mtproto.User
	for _, u := range got.GetUsers() {
		if u.GetId() == 777001 {
			ai = u
		}
	}
	if ai == nil {
		t.Fatal("resolved users did not include 777001")
	}
	if ai.GetFirstName().GetValue() != "Tellomi AI" {
		t.Fatalf("first_name = %q", ai.GetFirstName().GetValue())
	}
	if ai.GetUsername().GetValue() != "tellomi" {
		t.Fatalf("username = %q", ai.GetUsername().GetValue())
	}
}
