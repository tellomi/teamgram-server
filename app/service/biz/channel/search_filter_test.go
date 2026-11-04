package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestChannelSearchFilterDateAndTopic(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	photo := mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{}).To_MessageMedia()
	msg := func(text string, date int32, media *mtproto.MessageMedia) *mtproto.Message {
		return mtproto.MakeTLMessage(&mtproto.Message{Message: text, Date: date, Media: media}).To_Message()
	}
	if _, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{
		{RandomID: 1, Message: msg("old text", 10, nil)},
		{RandomID: 2, Message: msg("new text", 50, nil)},
		{RandomID: 3, Message: msg("pic", 40, photo)},
	}); err != nil {
		t.Fatal(err)
	}
	found, err := svc.SearchInChannel(ctx, 1, ch.Id, ChannelSearch{
		Q: "text", MinDate: 20, Limit: 10,
	})
	if err != nil || len(found.GetMessages()) != 1 || found.GetMessages()[0].GetMessage() != "new text" {
		t.Fatalf("date %#v %v", found.GetMessages(), err)
	}
	photos, err := svc.SearchInChannel(ctx, 1, ch.Id, ChannelSearch{
		Filter: mtproto.MakeTLInputMessagesFilterPhotos(&mtproto.MessagesFilter{}).To_MessagesFilter(),
		Limit:  10,
	})
	if err != nil || len(photos.GetMessages()) != 1 || photos.GetMessages()[0].GetMessage() != "pic" {
		t.Fatalf("photos %#v %v", photos.GetMessages(), err)
	}

	root, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 4, Message: msg("root", 60, nil),
	}})
	if err != nil {
		t.Fatal(err)
	}
	rootID := root.Updates[0].GetMessage_MESSAGE().GetId()
	reply := mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
		ReplyToMsgId: rootID,
	}).To_MessageReplyHeader()
	if _, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 5, Message: mtproto.MakeTLMessage(&mtproto.Message{Message: "thread", Date: 61, ReplyTo: reply}).To_Message(),
	}}); err != nil {
		t.Fatal(err)
	}
	thread, err := svc.SearchInChannel(ctx, 1, ch.Id, ChannelSearch{TopMsgID: rootID, Limit: 10})
	if err != nil || len(thread.GetMessages()) != 2 {
		t.Fatalf("topic %#v %v", thread.GetMessages(), err)
	}
}
