package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestMegagroupOnlyAuthorCanEdit(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "EditRoom")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	rights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{
		EditMessages:   true,
		DeleteMessages: true,
	}).To_ChatAdminRights()
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 3, rights, "mod"); err != nil {
		t.Fatal(err)
	}

	sent, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{
		RandomID: 41,
		Message:  &mtproto.Message{Message: "hello", Date: 10},
	}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].GetMessage_MESSAGE().GetId()

	if _, err := svc.Edit(ctx, 1, ch.Id, 0, msgID, func(m *mtproto.Message) {
		m.Message = "creator"
	}); err != mtproto.ErrMessageAuthorRequired {
		t.Fatalf("creator edit: %v", err)
	}
	if _, err := svc.Edit(ctx, 3, ch.Id, 0, msgID, func(m *mtproto.Message) {
		m.Message = "admin"
	}); err != mtproto.ErrMessageAuthorRequired {
		t.Fatalf("admin edit: %v", err)
	}
	edited, err := svc.Edit(ctx, 2, ch.Id, 0, msgID, func(m *mtproto.Message) {
		m.Message = "author"
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := edited.Updates[0].GetMessage_MESSAGE().GetMessage(); got != "author" {
		t.Fatalf("author edit = %q", got)
	}
	if from := edited.Updates[0].GetMessage_MESSAGE().GetFromId().GetUserId(); from != 2 {
		t.Fatalf("from_id = %d", from)
	}

	pts, count, err := svc.Delete(ctx, 1, ch.Id, 0, []int32{msgID})
	if err != nil || pts == 0 || count != 1 {
		t.Fatalf("creator delete pts=%d count=%d err=%v", pts, count, err)
	}
}

func TestBroadcastAdminCanEditAnotherPost(t *testing.T) {
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
	rights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{
		PostMessages: true,
		EditMessages: true,
	}).To_ChatAdminRights()
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 2, rights, ""); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 42,
		Message:  &mtproto.Message{Message: "post", Date: 10},
	}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].GetMessage_MESSAGE().GetId()
	edited, err := svc.Edit(ctx, 2, ch.Id, 0, msgID, func(m *mtproto.Message) {
		m.Message = "edited"
	})
	if err != nil {
		t.Fatalf("broadcast admin edit: %v", err)
	}
	if got := edited.Updates[0].GetMessage_MESSAGE().GetMessage(); got != "edited" {
		t.Fatalf("broadcast edit = %q", got)
	}
}
