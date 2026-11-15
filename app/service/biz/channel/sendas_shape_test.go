package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestMegagroupKeepsChannelSendAs(t *testing.T) {
	svc, ctx := newTest(t)
	group, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	news, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "news", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	newsID := news.Chats[0].Id
	sent, err := svc.Send(ctx, 1, group.Chats[0].Id, 0, []Outgoing{{
		RandomID: 1,
		Message: &mtproto.Message{
			Message: "as channel",
			FromId:  mtproto.MakePeerChannel(newsID),
			Date:    3,
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if sent.Updates[0].Message_MESSAGE.GetFromId().GetChannelId() != newsID {
		t.Fatalf("from %#v", sent.Updates[0].Message_MESSAGE.GetFromId())
	}
	plain, err := svc.Send(ctx, 1, group.Chats[0].Id, 0, []Outgoing{{
		RandomID: 2,
		Message:  &mtproto.Message{Message: "as me", Date: 4},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if plain.Updates[0].Message_MESSAGE.GetFromId().GetUserId() != 1 {
		t.Fatalf("user from %#v", plain.Updates[0].Message_MESSAGE.GetFromId())
	}
}
