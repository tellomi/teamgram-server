package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestDeleteChannelHistoryHidesItForThatMember(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{
		{RandomID: 1, Message: mtproto.MakeTLMessage(&mtproto.Message{Message: "early", Date: 10}).To_Message()},
		{RandomID: 2, Message: mtproto.MakeTLMessage(&mtproto.Message{Message: "later", Date: 40}).To_Message()},
	})
	if err != nil {
		t.Fatal(err)
	}
	later := sent.Updates[len(sent.Updates)-1].GetMessage_MESSAGE().GetId()
	if _, _, err := svc.DeleteHistory(ctx, 2, 0, in, later, true); err != mtproto.ErrChatAdminRequired {
		t.Fatalf("member revoke: %v", err)
	}
	pts, count, err := svc.DeleteHistory(ctx, 2, 0, in, later, false)
	if err != nil || pts == 0 {
		t.Fatalf("clear pts=%d count=%d err=%v", pts, count, err)
	}
	mine, err := svc.History(ctx, 2, in, HistoryQuery{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range mine.GetMessages() {
		if m.GetMessage() == "early" || m.GetMessage() == "later" {
			t.Fatalf("member still sees %q", m.GetMessage())
		}
	}
	theirs, err := svc.History(ctx, 1, in, HistoryQuery{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	var saw bool
	for _, m := range theirs.GetMessages() {
		if m.GetMessage() == "later" {
			saw = true
		}
	}
	if !saw {
		t.Fatal("creator lost the message")
	}
	if _, n, err := svc.DeleteByDate(ctx, 1, 0, in, 30, 0); err != nil || n < 1 {
		t.Fatalf("date delete n=%d err=%v", n, err)
	}
	theirs, err = svc.History(ctx, 1, in, HistoryQuery{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	var early bool
	for _, m := range theirs.GetMessages() {
		if m.GetMessage() == "later" {
			t.Fatal("dated message remained")
		}
		if m.GetMessage() == "early" {
			early = true
		}
	}
	if !early {
		t.Fatal("early message was deleted with the later one")
	}
}
