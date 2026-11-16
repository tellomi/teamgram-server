package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

func TestMayManageGiftCollection(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Gifts")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	post := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{PostMessages: true}).To_ChatAdminRights()
	inviteOnly := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{InviteUsers: true}).To_ChatAdminRights()
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 3, post, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 2, inviteOnly, ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.MayManageGiftCollection(ctx, 1, ch.Id); err != nil {
		t.Fatal(err)
	}
	if err := svc.MayManageGiftCollection(ctx, 3, ch.Id); err != nil {
		t.Fatal(err)
	}
	if status.Convert(svc.MayManageGiftCollection(ctx, 2, ch.Id)).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("invite-only admin: %v", svc.MayManageGiftCollection(ctx, 2, ch.Id))
	}
	if status.Convert(svc.MayManageGiftCollection(ctx, 9, ch.Id)).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatal("outsider")
	}
	if status.Convert(svc.MayManageGiftCollection(ctx, 1, ch.Id+99)).Message() != "PEER_ID_INVALID" {
		t.Fatal("missing channel")
	}
}
