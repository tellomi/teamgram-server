package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestForumTopicDeleteDoesNotWipeTheChannel(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Forum", Megagroup: true, Forum: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	topics, _, err := svc.ListTopics(ctx, 1, ch.Id, "", 20, 0, 0, 0)
	if err != nil || len(topics) == 0 || topics[0].GetTitle() != "General" || !topics[0].GetHidden() {
		t.Fatalf("general topic %#v %v", topics, err)
	}
	_, hello, err := svc.CreateTopic(ctx, 1, 0, ch.Id, "Hello", 0x6FB9F0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if hello.ID == 0 || hello.ID != hello.TopMessage {
		t.Fatalf("topic id should be the creation message, got %#v", hello)
	}
	icon := int64(42)
	hidden := false
	edited, err := svc.EditTopic(ctx, 1, ch.Id, hello.ID, "Hello!", nil, &icon, &hidden)
	if err != nil || edited.IconEmoji != 42 || edited.Title != "Hello!" {
		t.Fatalf("edit %#v %v", edited, err)
	}

	reply := mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
		ReplyToMsgId: hello.ID,
		ReplyToTopId: &wrapperspb.Int32Value{Value: hello.ID},
		ForumTopic:   true,
	}).To_MessageReplyHeader()
	inside, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 11,
		Message:  &mtproto.Message{Message: "inside", Date: 10, ReplyTo: reply},
	}})
	if err != nil {
		t.Fatal(err)
	}
	insideID := inside.Updates[0].GetMessage_MESSAGE().GetId()
	outside, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 12,
		Message:  &mtproto.Message{Message: "outside", Date: 11},
	}})
	if err != nil {
		t.Fatal(err)
	}
	outsideID := outside.Updates[0].GetMessage_MESSAGE().GetId()

	if _, _, err := svc.DeleteTopicHistory(ctx, 1, 0, ch.Id, 999999); err != mtproto.ErrTopicIdInvalid {
		t.Fatalf("unknown topic: %v", err)
	}
	pts, count, err := svc.DeleteTopicHistory(ctx, 1, 0, ch.Id, hello.ID)
	if err != nil || count < 1 || pts == 0 {
		t.Fatalf("delete pts=%d count=%d err=%v", pts, count, err)
	}
	hist, err := svc.History(ctx, 1, inCh(ch), HistoryQuery{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	var sawOutside, sawInside bool
	for _, m := range hist.GetMessages() {
		if m.GetId() == outsideID {
			sawOutside = true
		}
		if m.GetId() == insideID {
			sawInside = true
		}
	}
	if !sawOutside || sawInside {
		t.Fatalf("outside=%v inside=%v ids=%d/%d", sawOutside, sawInside, outsideID, insideID)
	}

	replies, err := svc.Replies(ctx, 1, ch.Id, hello.ID, HistoryQuery{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range replies.GetMessages() {
		if m.GetId() == insideID {
			t.Fatal("deleted topic message still in replies")
		}
	}
}

func TestReorderPinnedTopicsAndUnpinAll(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Forum", Megagroup: true, Forum: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	_, a, err := svc.CreateTopic(ctx, 1, 0, ch.Id, "Alpha", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, b, err := svc.CreateTopic(ctx, 1, 0, ch.Id, "Beta", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ReorderPinnedTopics(ctx, 1, ch.Id, []int32{b.ID, a.ID}, true); err != nil {
		t.Fatal(err)
	}
	listed, _, err := svc.ListTopics(ctx, 1, ch.Id, "", 10, 0, 0, 0)
	if err != nil || len(listed) < 2 {
		t.Fatalf("list %#v %v", listed, err)
	}
	if listed[0].GetId() != b.ID || !listed[0].GetPinned() || listed[1].GetId() != a.ID {
		t.Fatalf("pin order %#v %#v", listed[0], listed[1])
	}
	page, _, err := svc.ListTopics(ctx, 1, ch.Id, "", 10, 0, 0, b.ID)
	if err != nil || len(page) == 0 || page[0].GetId() == b.ID {
		t.Fatalf("offset page %#v %v", page, err)
	}

	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 3, Message: &mtproto.Message{Message: "pin me", Date: 4}}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].GetMessage_MESSAGE().GetId()
	if _, err := svc.PinMessage(ctx, 1, 0, ch.Id, msgID, false); err != nil {
		t.Fatal(err)
	}
	pts, count, err := svc.UnpinAll(ctx, 1, 0, ch.Id)
	if err != nil || count != 1 || pts == 0 {
		t.Fatalf("unpin pts=%d count=%d err=%v", pts, count, err)
	}
	full, err := svc.GetFullChannel(ctx, 1, inCh(ch))
	if err != nil || full.FullChat.GetPinnedMsgId().GetValue() != 0 {
		t.Fatalf("still pinned %#v %v", full.GetFullChat().GetPinnedMsgId(), err)
	}
	if _, count, err = svc.UnpinAll(ctx, 1, 0, ch.Id); err != nil || count != 0 {
		t.Fatalf("second unpin count=%d err=%v", count, err)
	}
}
