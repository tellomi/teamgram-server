package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestMegagroupEditRequiresAuthor(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	editRights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{
		EditMessages:   true,
		DeleteMessages: true,
	}).To_ChatAdminRights()
	if _, err = svc.EditAdmin(ctx, 1, 0, in, 3, editRights, ""); err != nil {
		t.Fatal(err)
	}

	sent, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{
		RandomID: 11,
		Message:  &mtproto.Message{Message: "from member", Date: 20},
	}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].GetMessage_MESSAGE().GetId()
	change := func(m *mtproto.Message) { m.Message = "rewritten" }

	if _, err = svc.Edit(ctx, 1, ch.Id, 0, msgID, change); err != mtproto.ErrMessageAuthorRequired {
		t.Fatalf("owner edit: %v", err)
	}
	if _, err = svc.Edit(ctx, 3, ch.Id, 0, msgID, change); err != mtproto.ErrMessageAuthorRequired {
		t.Fatalf("admin with edit_messages: %v", err)
	}
	edited, err := svc.Edit(ctx, 2, ch.Id, 0, msgID, change)
	if err != nil {
		t.Fatal(err)
	}
	if edited.Updates[0].GetMessage_MESSAGE().GetMessage() != "rewritten" {
		t.Fatalf("author edit = %q", edited.Updates[0].GetMessage_MESSAGE().GetMessage())
	}
	if _, _, err = svc.Delete(ctx, 1, ch.Id, 0, []int32{msgID}); err != nil {
		t.Fatalf("owner delete: %v", err)
	}
}

func TestChannelPostEditNeedsEditMessages(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.EditAdmin(ctx, 1, 0, in, 2, mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{
		PostMessages: true,
		EditMessages: true,
	}).To_ChatAdminRights(), ""); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 12,
		Message:  &mtproto.Message{Message: "post", Date: 21},
	}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].GetMessage_MESSAGE().GetId()
	change := func(m *mtproto.Message) { m.Message = "post!" }
	if _, err = svc.Edit(ctx, 3, ch.Id, 0, msgID, change); err != mtproto.ErrMessageAuthorRequired {
		t.Fatalf("subscriber edit: %v", err)
	}
	edited, err := svc.Edit(ctx, 2, ch.Id, 0, msgID, change)
	if err != nil {
		t.Fatal(err)
	}
	if edited.Updates[0].GetMessage_MESSAGE().GetMessage() != "post!" {
		t.Fatalf("editor = %q", edited.Updates[0].GetMessage_MESSAGE().GetMessage())
	}
}
