package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

func TestChangeInfoRights(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Room", Megagroup: true, About: "start"})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.EditTitle(ctx, 2, 0, in, "Stolen"); status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("member title: %v", err)
	}
	if _, err := svc.EditAbout(ctx, 2, 0, in, "nope"); status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("member about: %v", err)
	}
	if _, err := svc.EditPhoto(ctx, 2, 0, in, nil); status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("member photo: %v", err)
	}
	if got := mustChannel(t, svc, ctx, ch.Id).Title; got != "Room" {
		t.Fatalf("title changed to %q", got)
	}

	plain := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{InviteUsers: true}).To_ChatAdminRights()
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 3, plain, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditTitle(ctx, 3, 0, in, "Still"); status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("admin without change_info: %v", err)
	}
	with := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{ChangeInfo: true}).To_ChatAdminRights()
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 3, with, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditTitle(ctx, 3, 0, in, "Renamed"); err != nil {
		t.Fatal(err)
	}

	open := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{SendStickers: true}).To_ChatBannedRights()
	if _, err := svc.EditDefaultBanned(ctx, 1, 0, in, open); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditTitle(ctx, 2, 0, in, "Member title"); err != nil {
		t.Fatalf("member should edit when change_info is not banned: %v", err)
	}
	ban := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{ChangeInfo: true}).To_ChatBannedRights()
	if _, err := svc.EditBanned(ctx, 1, 0, in, 2, ban); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditTitle(ctx, 2, 0, in, "Banned"); status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("per-user change_info ban: %v", err)
	}

	news, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	nin := input(news.Chats[0].Id, news.Chats[0].GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.Invite(ctx, 1, 0, nin, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditDefaultBanned(ctx, 1, 0, nin, open); status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("broadcast default rights: %v", err)
	}
	if _, err := svc.EditTitle(ctx, 2, 0, nin, "Hijack"); status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("channel subscriber title: %v", err)
	}
	if _, err := svc.EditTitle(ctx, 1, 0, nin, "Bulletin"); err != nil {
		t.Fatal(err)
	}
}
