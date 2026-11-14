package service

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/channels/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestInviteToChannelPrivacyAndBlock(t *testing.T) {
	prev := channel.Shared()
	channel.Use(channel.New(channel.NewMemoryStore()))
	t.Cleanup(func() { channel.Use(prev) })

	ctx := channelCtx(1)
	created, err := channel.Shared().Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "Room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	inCh := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  ch.Id,
		AccessHash: ch.GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputChannel()
	users := &channelInviteUser{allow: true}
	s := &Service{svcCtx: &svc.ServiceContext{UserClient: users}}
	one := []*mtproto.InputUser{
		mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2, AccessHash: 1}).To_InputUser(),
	}

	users.allow = false
	if _, err = s.ChannelsInviteToChannel199F3A6C(ctx, &mtproto.TLChannelsInviteToChannel199F3A6C{
		Channel: inCh, Users: one,
	}); status.Convert(err).Message() != "USER_PRIVACY_RESTRICTED" {
		t.Fatalf("privacy deny: %v", err)
	}

	users.allow = true
	users.blocked = true
	if _, err = s.ChannelsInviteToChannel199F3A6C(ctx, &mtproto.TLChannelsInviteToChannel199F3A6C{
		Channel: inCh, Users: one,
	}); status.Convert(err).Message() != "USER_PRIVACY_RESTRICTED" {
		t.Fatalf("block deny: %v", err)
	}

	users.blocked = false
	got, err := s.ChannelsInviteToChannelC9E33D54(ctx, &mtproto.TLChannelsInviteToChannelC9E33D54{
		Channel: inCh, Users: one,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetMissingInvitees()) != 0 {
		t.Fatalf("allowed invite missing %+v", got.GetMissingInvitees())
	}
}

func channelCtx(userID int64) context.Context {
	out, err := rpcmd.RpcMetadataToOutgoing(context.Background(), &rpcmd.RpcMetadata{UserId: userID, PermAuthKeyId: 9})
	if err != nil {
		panic(err)
	}
	md, _ := metadata.FromOutgoingContext(out)
	return metadata.NewIncomingContext(context.Background(), md)
}

type channelInviteUser struct {
	userclient.UserClient
	allow   bool
	blocked bool
}

func (u *channelInviteUser) UserCheckPrivacy(context.Context, *user.TLUserCheckPrivacy) (*mtproto.Bool, error) {
	if u.allow {
		return mtproto.BoolTrue, nil
	}
	return mtproto.BoolFalse, nil
}

func (u *channelInviteUser) UserBlockedEitherWay(context.Context, *user.TLUserCheckBlockUserList) (*user.Vector_Long, error) {
	if u.blocked {
		return &user.Vector_Long{Datas: []int64{2}}, nil
	}
	return &user.Vector_Long{}, nil
}
