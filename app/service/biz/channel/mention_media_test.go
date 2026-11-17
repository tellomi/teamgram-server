package channel

import (
	"context"
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

func TestReadMentionsClearsBadgeWithoutReadingHistory(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "MentionBadge")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	first := sendMention(t, svc, ctx, ch.Id, 1, "hi")
	before := dialogMention(t, svc, ctx, 1, ch.Id)
	if before.UnreadMentions < 1 || before.Unread < 1 {
		t.Fatalf("before mentions=%d unread=%d", before.UnreadMentions, before.Unread)
	}
	affected, err := svc.ReadMentions(ctx, 1, ch.Id, 0)
	if err != nil || affected.GetOffset() != 0 {
		t.Fatalf("read %v %+v", err, affected)
	}
	after := dialogMention(t, svc, ctx, 1, ch.Id)
	if after.UnreadMentions != 0 || after.Unread < 1 {
		t.Fatalf("after mentions=%d unread=%d", after.UnreadMentions, after.Unread)
	}
	unread, err := svc.UnreadMentions(ctx, 1, ch.Id, 0, 0, 20, 0, 0)
	if err != nil || unread.GetCount() != 0 {
		t.Fatalf("list %+v %v", unread, err)
	}
	second := sendMention(t, svc, ctx, ch.Id, 2, "yo")
	if second <= first {
		t.Fatalf("ids %d %d", first, second)
	}
	later := dialogMention(t, svc, ctx, 1, ch.Id)
	if later.UnreadMentions != 1 {
		t.Fatalf("later mention %d", later.UnreadMentions)
	}
	if _, err = svc.ReadMentions(ctx, 99, ch.Id, 0); err != mtproto.ErrChannelPrivate {
		t.Fatalf("outsider %v", err)
	}
}

func TestReadMentionsTopicLeavesOtherTopics(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "MentionTopic")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	topicMsg := sendMention(t, svc, ctx, ch.Id, 1, "one")
	otherMsg := sendMention(t, svc, ctx, ch.Id, 2, "two")
	f := svc.features(ctx, ch.Id)
	f.MsgTopic[topicMsg] = 7
	if err := svc.saveFeatures(ctx, ch.Id, f); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReadMentions(ctx, 1, ch.Id, 7); err != nil {
		t.Fatal(err)
	}
	left := dialogMention(t, svc, ctx, 1, ch.Id)
	if left.UnreadMentions != 1 {
		t.Fatalf("topic read cleared %d", left.UnreadMentions)
	}
	unread, err := svc.UnreadMentions(ctx, 1, ch.Id, 0, 0, 20, 0, 0)
	if err != nil || unread.GetCount() != 1 || unread.Messages[0].GetId() != otherMsg {
		t.Fatalf("remaining %+v %v", unread, err)
	}
}

func sendMention(t *testing.T, svc *Service, ctx context.Context, channelID, random int64, text string) int32 {
	t.Helper()
	sent, err := svc.Send(ctx, 2, channelID, 0, []Outgoing{{RandomID: random, Message: &mtproto.Message{
		Message: text,
		Entities: []*mtproto.MessageEntity{
			mtproto.MakeTLMessageEntityMentionName(&mtproto.MessageEntity{
				Offset: 0, Length: int32(len(text)), UserId_INT64: 1,
			}).To_MessageEntity(),
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return sentChannelMessage(t, sent).GetId()
}

func dialogMention(t *testing.T, svc *Service, ctx context.Context, userID, channelID int64) DialogRow {
	t.Helper()
	rows, err := svc.DialogsFor(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ChannelID == channelID {
			return row
		}
	}
	t.Fatalf("no dialog %d", channelID)
	return DialogRow{}
}
