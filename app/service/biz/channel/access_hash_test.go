package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/accesshash"
	"google.golang.org/grpc/status"
)

func TestLookupOffRejectsWrongHashWhenRowIsZero(t *testing.T) {
	svc, ctx := newTest(t)
	up, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Zero", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := svc.store.GetChannel(ctx, up.Chats[0].Id)
	if err != nil || ch == nil {
		t.Fatal(err)
	}
	ch.AccessHash = 0
	if err := svc.store.SaveChannel(ctx, ch); err != nil {
		t.Fatal(err)
	}
	defer accesshash.SetMode(accesshash.ModeOff)()
	if _, err := svc.GetFullChannel(ctx, 1, input(ch.ID, 5)); status.Convert(err).Message() != "CHANNEL_INVALID" {
		t.Fatalf("non-zero against stored 0: %v", err)
	}
	if _, err := svc.GetFullChannel(ctx, 1, input(ch.ID, 0)); err != nil {
		t.Fatal(err)
	}
}

func TestLookupRejectsZeroHashWhenEnforced(t *testing.T) {
	svc, ctx := newTest(t)
	up, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Feed", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := up.Chats[0]
	hash := ch.GetAccessHash_FLAGINT64().GetValue()
	if hash == 0 {
		t.Fatal("create issued access_hash 0")
	}

	defer accesshash.SetMode(accesshash.ModeOff)()
	if _, err := svc.GetFullChannel(ctx, 1, input(ch.Id, 0)); err != nil {
		t.Fatal(err)
	}

	defer accesshash.SetMode(accesshash.ModeEnforce)()
	_, err = svc.GetFullChannel(ctx, 1, input(ch.Id, 0))
	if status.Convert(err).Message() != "CHANNEL_INVALID" {
		t.Fatalf("zero hash: %v", err)
	}
	_, err = svc.GetFullChannel(ctx, 1, input(ch.Id, hash+1))
	if status.Convert(err).Message() != "CHANNEL_INVALID" {
		t.Fatalf("wrong hash: %v", err)
	}
	if _, err := svc.GetFullChannel(ctx, 1, input(ch.Id, hash)); err != nil {
		t.Fatal(err)
	}
}

func TestVisibleMessageHidesPrivatePosts(t *testing.T) {
	svc, ctx := newTest(t)
	up, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Feed", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := up.Chats[0]
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 3,
		Message:  &mtproto.Message{PredicateName: mtproto.Predicate_message, Message: "hi"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].Message_MESSAGE.Id
	if _, ok, err := svc.VisibleMessage(ctx, 1, ch.Id, msgID); err != nil || !ok {
		t.Fatalf("creator ok=%v err=%v", ok, err)
	}
	if _, ok, err := svc.VisibleMessage(ctx, 9, ch.Id, msgID); err != nil || ok {
		t.Fatalf("stranger ok=%v err=%v", ok, err)
	}
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.UpdateUsername(ctx, 1, 0, in, "feedvisible"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := svc.VisibleMessage(ctx, 9, ch.Id, msgID); err != nil || !ok {
		t.Fatalf("public ok=%v err=%v", ok, err)
	}
}
