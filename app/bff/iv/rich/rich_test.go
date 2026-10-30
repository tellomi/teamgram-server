package rich

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"google.golang.org/grpc/status"
)

func TestLoadRichMessage(t *testing.T) {
	svc := channel.New(channel.NewMemoryStore())
	channel.Use(svc)
	ctx := context.Background()
	up, err := svc.Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "Paper", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := up.Chats[0]
	if _, err := svc.Send(ctx, 1, ch.Id, 0, []channel.Outgoing{{
		RandomID: 4,
		Message:  &mtproto.Message{Message: "article", Date: 9},
	}}); err != nil {
		t.Fatal(err)
	}
	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId:  ch.Id,
		AccessHash: ch.GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputPeer()
	page, err := Load(ctx, 1, peer, 2)
	if err != nil || len(page.GetMessages()) != 1 || page.Messages[0].GetMessage() != "article" {
		t.Fatalf("page=%v err=%v", page, err)
	}
	if _, err := Load(ctx, 1, peer, 99); status.Convert(err).Message() != "MESSAGE_ID_INVALID" {
		t.Fatal(err)
	}
	user := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 2, AccessHash: 1}).To_InputPeer()
	if _, err := Load(ctx, 1, user, 1); status.Convert(err).Message() != "MESSAGE_ID_INVALID" {
		t.Fatal(err)
	}
	if _, err := Load(ctx, 0, peer, 1); status.Convert(err).Message() != "AUTH_KEY_UNREGISTERED" {
		t.Fatal(err)
	}
}
