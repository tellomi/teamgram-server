package channel

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

func TestDirectMessagesLocationAndThread(t *testing.T) {
	svc, ctx := newTest(t)
	up, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := up.Chats[0]
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{RandomID: 1, Message: &mtproto.Message{Message: "no"}}}); status.Convert(err).Message() != "CHAT_WRITE_FORBIDDEN" {
		t.Fatalf("before: %v", err)
	}
	if _, err := svc.UpdatePaidMessagesPrice(ctx, 1, 0, in, 0, true); err != nil {
		t.Fatal(err)
	}
	stored, err := svc.store.GetChannel(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	view := svc.ChatFor(ctx, 2, stored)
	if !view.GetMonoforum() || view.GetLinkedMonoforumId().GetValue() != ch.Id {
		t.Fatalf("monoforum %#v", view)
	}
	if _, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{RandomID: 2, Message: &mtproto.Message{Message: "hi"}}}); err != nil {
		t.Fatal(err)
	}

	geo := megagroup(t, svc, ctx, "Place")
	if err := svc.EditLocation(ctx, 1, inCh(geo), 31.2, 121.4, "上海"); err != nil {
		t.Fatal(err)
	}
	full, err := svc.GetFullChannel(ctx, 1, inCh(geo))
	if err != nil || !full.GetFullChat().GetCanSetLocation() {
		t.Fatalf("creator location %#v %v", full, err)
	}
	prevJoin := LookupJoinAs
	LookupJoinAs = func(context.Context, int64, int64) *mtproto.Peer {
		return &mtproto.Peer{PredicateName: mtproto.Predicate_peerUser, UserId: 9}
	}
	t.Cleanup(func() { LookupJoinAs = prevJoin })
	joined, err := svc.GetFullChannel(ctx, 1, inCh(geo))
	if err != nil || joined.GetFullChat().GetGroupcallDefaultJoinAs().GetUserId() != 9 {
		t.Fatalf("join as %#v %v", joined, err)
	}

	group := megagroup(t, svc, ctx, "Talk")
	root, err := svc.Send(ctx, 1, group.Id, 0, []Outgoing{{RandomID: 3, Message: &mtproto.Message{Message: "root"}}})
	if err != nil {
		t.Fatal(err)
	}
	rootID := root.Updates[0].GetMessage_MESSAGE().GetId()
	reply, err := svc.Send(ctx, 1, group.Id, 0, []Outgoing{{RandomID: 4, Message: &mtproto.Message{
		Message: "reply",
		ReplyTo: mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{ReplyToMsgId: rootID, ReplyToMsgId_INT32: rootID}).To_MessageReplyHeader(),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	replyID := reply.Updates[0].GetMessage_MESSAGE().GetId()
	got, err := svc.GetMessages(ctx, 1, inCh(group), []int32{rootID})
	if err != nil || len(got.Messages) != 1 || got.Messages[0].GetReplies().GetReplies() != 1 {
		t.Fatalf("replies %#v %v", got, err)
	}
	if err := svc.ReadThread(ctx, 1, group.Id, rootID, replyID); err != nil {
		t.Fatal(err)
	}
	again, err := svc.GetMessages(ctx, 1, inCh(group), []int32{rootID})
	if err != nil || again.Messages[0].GetReplies().GetReadMaxId().GetValue() != replyID {
		t.Fatalf("read max %#v %v", again, err)
	}
	if _, err := svc.Invite(ctx, 1, 0, inCh(group), []int64{3}, nil); err != nil {
		t.Fatal(err)
	}
	other, err := svc.GetMessages(ctx, 3, inCh(group), []int32{rootID})
	if err != nil || other.Messages[0].GetReplies().GetReadMaxId() != nil {
		t.Fatalf("other reader %#v %v", other, err)
	}
}
