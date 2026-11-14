package payments_helper

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"google.golang.org/grpc/status"
)

func TestBoostsListRequiresAdmin(t *testing.T) {
	prev := channel.Shared()
	svc := channel.New(channel.NewMemoryStore())
	channel.Use(svc)
	t.Cleanup(func() { channel.Use(prev) })

	ctx := context.Background()
	created, err := svc.Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "Boosts", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: ch.Id, AccessHash: ch.GetAccessHash_FLAGINT64().GetValue()}
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	rights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{BanUsers: true}).To_ChatAdminRights()
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 3, rights, ""); err != nil {
		t.Fatal(err)
	}
	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId:  ch.Id,
		AccessHash: ch.GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputPeer()
	req := &mtproto.TLPremiumGetBoostsList{Peer: peer, Limit: 10}
	if _, err := BoostsList(9, req); status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("outsider: %v", err)
	}
	if _, err := BoostsList(2, req); status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("member: %v", err)
	}
	if _, err := BoostsList(1, req); err != nil {
		t.Fatalf("creator: %v", err)
	}
	if _, err := BoostsList(3, req); err != nil {
		t.Fatalf("admin: %v", err)
	}
	chat := mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 4}).To_InputPeer()
	if _, err := BoostsList(1, &mtproto.TLPremiumGetBoostsList{Peer: chat, Limit: 10}); status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("basic group: %v", err)
	}
}
