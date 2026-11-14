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
	for _, userID := range []int64{1, 2} {
		rows, err = svc.DialogsFor(ctx, userID)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].TopMessage != third.Updates[0].GetMessage_MESSAGE().GetId() {
			t.Fatalf("user %d dialogs %+v", userID, rows)
		}
	}
}

func TestDeleteLatestBroadcastPostKeepsPreviousDialogTop(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Feed", Broadcast: true})
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
	if len(rows) != 1 || rows[0].TopMessage != olderID || rows[0].Date != 10 {
		t.Fatalf("dialogs %+v, want top %d", rows, olderID)
	}
}

func TestDeleteMiddleMessageKeepsNewestDialogTop(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	var ids []int32
	for i, text := range []string{"a", "b", "c"} {
		sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
			RandomID: int64(i + 1),
			Message:  &mtproto.Message{Message: text, Date: int32(10 * (i + 1))},
		}})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, sent.Updates[0].GetMessage_MESSAGE().GetId())
	}
	if _, _, err := svc.Delete(ctx, 1, ch.Id, 0, []int32{ids[1]}); err != nil {
		t.Fatal(err)
	}
	rows, err := svc.DialogsFor(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].TopMessage != ids[2] || rows[0].Date != 30 {
		t.Fatalf("dialogs %+v, want top %d", rows, ids[2])
	}
}

func TestDeleteEveryMessageKeepsMembershipDialog(t *testing.T) {
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
	for i := 0; i < 2; i++ {
		if _, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
			RandomID: int64(i + 1),
			Message:  &mtproto.Message{Message: "x", Date: int32(10 * (i + 1))},
		}}); err != nil {
			t.Fatal(err)
		}
	}
	// Creation also posts a service message. Every live id has to go,
	// including that one, before the dialog top is empty.
	msgs, err := svc.store.ListMessages(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int32
	var lastID int32
	for _, m := range msgs {
		if m == nil || m.Deleted {
			continue
		}
		ids = append(ids, m.ID)
		if m.ID > lastID {
			lastID = m.ID
		}
	}
	if _, _, err := svc.Delete(ctx, 1, ch.Id, 0, ids); err != nil {
		t.Fatal(err)
	}
	for _, userID := range []int64{1, 2} {
		rows, err := svc.DialogsFor(ctx, userID)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].ChannelID != ch.Id || rows[0].TopMessage != 0 {
			t.Fatalf("user %d dialogs %+v", userID, rows)
		}
	}
	stored, err := svc.store.GetChannel(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.TopMessage != lastID {
		t.Fatalf("id counter %d, want %d", stored.TopMessage, ids[len(ids)-1])
	}
}
