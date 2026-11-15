package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestMegagroupMentionFlagAndVoiceUnread(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Mentions", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := inCh(ch)
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 1, Message: &mtproto.Message{Message: "root", Date: 10}}}); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{RandomID: 2, Message: &mtproto.Message{
		Message: "hi",
		Date:    11,
		Entities: []*mtproto.MessageEntity{
			mtproto.MakeTLMessageEntityMentionName(&mtproto.MessageEntity{Offset: 0, Length: 2, UserId_INT64: 1}).To_MessageEntity(),
		},
		Media: mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
			TtlSeconds: wrapperspb.Int32(5),
			Document:   mtproto.MakeTLDocument(&mtproto.Document{Id: 9, MimeType: "audio/ogg", Attributes: []*mtproto.DocumentAttribute{mtproto.MakeTLDocumentAttributeAudio(&mtproto.DocumentAttribute{Voice: true}).To_DocumentAttribute()}}).To_Document(),
		}).To_MessageMedia(),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	id := posted(sent).GetMessage_MESSAGE().GetId()
	hist, err := svc.History(ctx, 1, in, HistoryQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var msg *mtproto.Message
	for _, m := range hist.GetMessages() {
		if m.GetId() == id {
			msg = m
		}
	}
	if msg == nil || !msg.GetMentioned() || !msg.GetMediaUnread() {
		t.Fatalf("viewer message mentioned=%v media_unread=%v", msg.GetMentioned(), msg.GetMediaUnread())
	}
	rows, err := svc.DialogsFor(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	var mentions int32
	for _, row := range rows {
		if row.ChannelID == ch.Id {
			mentions = row.UnreadMentions
		}
	}
	if mentions < 1 {
		t.Fatalf("unread mentions %d", mentions)
	}
	if err = svc.ReadContents(ctx, 1, in, []int32{id}); err != nil {
		t.Fatal(err)
	}
	hist, err = svc.History(ctx, 1, in, HistoryQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range hist.GetMessages() {
		if m.GetId() == id && m.GetMediaUnread() {
			t.Fatal("media stayed unread")
		}
	}
}
