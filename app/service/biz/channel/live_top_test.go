package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestDeleteLatestMessageKeepsPreviousDialogTop(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.AccessHash_FLAGINT64.Value)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	first, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 1,
		Message:  &mtproto.Message{Message: "older", Date: 10},
	}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 2,
		Message:  &mtproto.Message{Message: "newer", Date: 20},
	}})
	if err != nil {
		t.Fatal(err)
	}
	olderID := first.Updates[0].GetMessage_MESSAGE().GetId()
	newerID := second.Updates[0].GetMessage_MESSAGE().GetId()
	if _, _, err := svc.Delete(ctx, 1, ch.Id, 0, []int32{newerID}); err != nil {
		t.Fatal(err)
	}
	rows, err := svc.DialogsFor(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("dialogs = %d", len(rows))
	}
	if rows[0].TopMessage != olderID {
		t.Fatalf("top_message = %d, want previous %d", rows[0].TopMessage, olderID)
	}
	if rows[0].Date != 10 {
		t.Fatalf("dialog date = %d", rows[0].Date)
	}
	// The id counter is not reused. The next post is a new id.
	third, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 3,
		Message:  &mtproto.Message{Message: "after", Date: 30},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if third.Updates[0].GetMessage_MESSAGE().GetId() == newerID {
		t.Fatal("deleted channel message id was reused")
	}
}
