package channel

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestForwardCardForANonMemberIsMin(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 10, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	id := created.Chats[0].GetId()

	member := svc.ForwardCard(ctx, 10, id)
	if member.GetPredicateName() != mtproto.Predicate_channel || member.GetMin() || member.GetTitle() != "News" {
		t.Fatalf("member card %#v", member)
	}

	outsider := svc.ForwardCard(ctx, 99, id)
	if outsider.GetPredicateName() != mtproto.Predicate_channel || !outsider.GetMin() || outsider.GetTitle() != "News" || outsider.GetId() != id {
		t.Fatalf("outsider card %#v", outsider)
	}
	if outsider.GetAccessHash_FLAGINT64().GetValue() == 0 {
		t.Fatal("min channel has no access hash")
	}
	buf := mtproto.NewEncodeBuf(256)
	if err := outsider.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}

	stored := loadChannel(t, svc, ctx, id)
	forbidden := svc.ChatFor(ctx, 99, stored)
	if forbidden.GetPredicateName() != mtproto.Predicate_channelForbidden {
		t.Fatalf("ChatFor outsider = %s", forbidden.GetPredicateName())
	}

	msg := &mtproto.Message{FwdFrom: &mtproto.MessageFwdHeader{FromId: mtproto.MakePeerChannel(id)}}
	chats := svc.AttachForwardCards(ctx, 99, []*mtproto.Message{msg}, []*mtproto.Chat{forbidden})
	if len(chats) != 1 || chats[0].GetPredicateName() != mtproto.Predicate_channel || !chats[0].GetMin() {
		t.Fatalf("attached %#v", chats)
	}
	again := svc.AttachForwardCards(ctx, 99, []*mtproto.Message{msg}, chats)
	if len(again) != 1 {
		t.Fatalf("duplicated %d", len(again))
	}
}

func loadChannel(t *testing.T, svc *Service, ctx context.Context, id int64) *Channel {
	t.Helper()
	ch, err := svc.store.GetChannel(ctx, id)
	if err != nil || ch == nil {
		t.Fatalf("channel %d: %v", id, err)
	}
	return ch
}
