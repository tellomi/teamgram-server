package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestAddForwardsCountsOnThePost(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 9,
		Message:  &mtproto.Message{Message: "post", Date: 10},
	}})
	if err != nil {
		t.Fatal(err)
	}
	id := sent.Updates[0].GetMessage_MESSAGE().GetId()
	if err = svc.AddForwards(ctx, ch.Id, []int32{id, id}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetMessages(ctx, 1, input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue()), []int32{id})
	if err != nil {
		t.Fatal(err)
	}
	if got.Messages[0].GetForwards().GetValue() != 2 {
		t.Fatalf("forwards = %d", got.Messages[0].GetForwards().GetValue())
	}
	views, err := svc.MessageViews(ctx, 1, ch.Id, []int32{id}, false)
	if err != nil {
		t.Fatal(err)
	}
	if views[0].GetForwards().GetValue() != 2 {
		t.Fatalf("views.forwards = %d", views[0].GetForwards().GetValue())
	}
}
