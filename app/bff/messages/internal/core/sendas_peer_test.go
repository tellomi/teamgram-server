package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/webpage"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

func TestSendAsPeerUsesAnOwnedChannel(t *testing.T) {
	prev := channel.Shared()
	svc := channel.New(channel.NewMemoryStore())
	channel.Use(svc)
	t.Cleanup(func() { channel.Use(prev) })

	ctx := context.Background()
	group, err := svc.Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	news, err := svc.Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "news", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	c := &MessagesCore{ctx: ctx, Logger: logx.WithContext(ctx), MD: &metadata.RpcMetadata{UserId: 1}}
	groupPeer := mtproto.FromInputPeer2(1, mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId: group.Chats[0].Id,
	}).To_InputPeer())
	got, err := c.sendAsPeer(groupPeer, mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId: news.Chats[0].Id,
	}).To_InputPeer())
	if err != nil || got.GetChannelId() != news.Chats[0].Id {
		t.Fatalf("send_as %#v %v", got, err)
	}
	if _, err := c.sendAsPeer(groupPeer, mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 9}).To_InputPeer()); status.Convert(err).Message() != "SEND_AS_PEER_INVALID" {
		t.Fatalf("other user: %v", err)
	}
	self, err := c.sendAsPeer(groupPeer, nil)
	if err != nil || self.GetUserId() != 1 {
		t.Fatalf("default %#v %v", self, err)
	}
}

func TestEditedChannelTextCanAttachAnInternalPage(t *testing.T) {
	media, pending := webpage.AttachOnSend(context.Background(), "看 https://tell.cc/addstickers/Twemoji", nil)
	if media.GetWebpage() == nil || pending {
		t.Fatalf("internal sticker page %#v pending %v", media, pending)
	}
	draft := &mtproto.Message{Message: "看 https://tell.cc/addstickers/Twemoji"}
	if shouldAutoAttachWebpage(false, draft.Media) {
		draft.Media = media
	}
	if draft.GetMedia().GetWebpage().GetType().GetValue() == "" {
		t.Fatal("edit draft has no webpage")
	}
	if shouldAutoAttachWebpage(true, nil) {
		t.Fatal("no_webpage still attaches")
	}
}
