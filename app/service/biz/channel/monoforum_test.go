package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestMonoforumAuthorAndRead(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 10, Title: "DM", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.AccessHash_FLAGINT64.Value)

	if _, err := svc.MonoforumAuthor(ctx, 10, in, 1); err != mtproto.ErrChannelInvalid {
		t.Fatalf("not a monoforum: %v", err)
	}
	if err := svc.SetMonoforum(ctx, ch.Id, true); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 10, ch.Id, 0, []Outgoing{{
		RandomID: 1,
		Message:  mtproto.MakeTLMessage(&mtproto.Message{Message: "hi"}).To_Message(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].GetMessage_MESSAGE().GetId()
	sender, err := svc.MonoforumAuthor(ctx, 10, in, msgID)
	if err != nil || sender != 10 {
		t.Fatalf("author %d %v", sender, err)
	}
	if _, err := svc.MonoforumAuthor(ctx, 10, in, 0); err != mtproto.ErrMsgIdInvalid {
		t.Fatalf("bad id: %v", err)
	}
	if _, err := svc.Invite(ctx, 10, 0, in, []int64{11}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MonoforumAuthor(ctx, 11, in, msgID); err != mtproto.ErrChatAdminRequired {
		t.Fatalf("member: %v", err)
	}

	parent := in
	if err := svc.ReadMonoforum(ctx, 10, parent, mtproto.PEER_USER, 11, 4); err != nil {
		t.Fatal(err)
	}
	if got := svc.MonoforumRead(ctx, 10, ch.Id, mtproto.PEER_USER, 11); got != 4 {
		t.Fatalf("read %d", got)
	}
	if err := svc.ReadMonoforum(ctx, 10, parent, mtproto.PEER_USER, 11, 2); err != nil {
		t.Fatal(err)
	}
	if got := svc.MonoforumRead(ctx, 10, ch.Id, mtproto.PEER_USER, 11); got != 4 {
		t.Fatalf("read moved backward to %d", got)
	}
	if err := svc.ReadMonoforum(ctx, 11, parent, mtproto.PEER_USER, 11, 1); err != mtproto.ErrChatAdminRequired {
		t.Fatalf("member read: %v", err)
	}
	other := input(ch.Id+99, 1)
	if err := svc.ReadMonoforum(ctx, 10, other, mtproto.PEER_USER, 11, 1); err != mtproto.ErrParentPeerInvalid {
		t.Fatalf("missing parent: %v", err)
	}
}
