package channel

import (
	"context"
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
	if err != nil || len(topics) == 0 || topics[0].GetTitle() != "General" || topics[0].GetId() != 1 || topics[0].GetHidden() {
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
	var sawRoot bool
	for _, m := range hist.GetMessages() {
		if m.GetId() == hello.ID {
			sawRoot = true
		}
	}
	if sawRoot {
		t.Fatal("topic creation message still in history")
	}
	left, _, err := svc.ListTopics(ctx, 1, ch.Id, "", 20, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, topic := range left {
		if topic.GetId() == hello.ID {
			t.Fatal("deleted topic still listed")
		}
	}
	if _, _, err := svc.DeleteTopicHistory(ctx, 1, 0, ch.Id, 1); err != mtproto.ErrTopicIdInvalid {
		t.Fatalf("general topic delete: %v", err)
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

func TestClosedTopicRejectsNonAdmins(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Forum", Megagroup: true, Forum: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	_, hello, err := svc.CreateTopic(ctx, 1, 0, ch.Id, "Hello", 0x6FB9F0, 0)
	if err != nil {
		t.Fatal(err)
	}
	closed := true
	if _, err := svc.EditTopic(ctx, 1, ch.Id, hello.ID, "", &closed, nil, nil); err != nil {
		t.Fatal(err)
	}
	reply := mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
		ReplyToMsgId: hello.ID,
		ReplyToTopId: &wrapperspb.Int32Value{Value: hello.ID},
		ForumTopic:   true,
	}).To_MessageReplyHeader()
	if _, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{
		RandomID: 21,
		Message:  &mtproto.Message{Message: "member", Date: 10, ReplyTo: reply},
	}}); err != mtproto.ErrTopicClosed {
		t.Fatalf("member post in closed topic: %v", err)
	}
	if _, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 22,
		Message:  &mtproto.Message{Message: "admin", Date: 11, ReplyTo: reply},
	}}); err != nil {
		t.Fatalf("admin post in closed topic: %v", err)
	}
}

func TestDeleteTopicEmitsChannelMessageDeletes(t *testing.T) {
	svc, ctx := newTest(t)
	var deleted []int32
	svc.SetHooks(Hooks{Push: func(_ context.Context, _ int64, _ []int64, updates *mtproto.Updates) {
		for _, u := range updates.GetUpdates() {
			if u.GetPredicateName() == mtproto.Predicate_updateDeleteChannelMessages {
				deleted = append(deleted, u.GetMessages()...)
			}
		}
	}})
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Forum", Megagroup: true, Forum: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	_, hello, err := svc.CreateTopic(ctx, 1, 0, ch.Id, "Hello", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	reply := mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
		ReplyToMsgId: hello.ID,
		ReplyToTopId: &wrapperspb.Int32Value{Value: hello.ID},
		ForumTopic:   true,
	}).To_MessageReplyHeader()
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 31,
		Message:  &mtproto.Message{Message: "inside", Date: 10, ReplyTo: reply},
	}})
	if err != nil {
		t.Fatal(err)
	}
	insideID := sent.Updates[0].GetMessage_MESSAGE().GetId()
	if _, _, err := svc.DeleteTopicHistory(ctx, 1, 0, ch.Id, hello.ID); err != nil {
		t.Fatal(err)
	}
	sawRoot, sawInside := false, false
	for _, id := range deleted {
		if id == hello.ID {
			sawRoot = true
		}
		if id == insideID {
			sawInside = true
		}
	}
	if !sawRoot || !sawInside {
		t.Fatalf("delete update ids=%v want %d and %d", deleted, hello.ID, insideID)
	}
}
