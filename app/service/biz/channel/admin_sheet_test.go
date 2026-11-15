package channel

import (
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func rights(change, linked, direct, ban bool) *mtproto.ChatAdminRights {
	return mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{
		ChangeInfo:           change,
		ManageLinkedPeers:    linked,
		ManageDirectMessages: direct,
		BanUsers:             ban,
	}).To_ChatAdminRights()
}

func TestAdminUsernameDiscussionHiddenDirectAndSubscriberList(t *testing.T) {
	svc, ctx := newTest(t)
	group := megagroup(t, svc, ctx, "Rights")
	gin := inCh(group)
	if _, err := svc.Invite(ctx, 1, 0, gin, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditAdmin(ctx, 1, 0, gin, 2, rights(true, true, false, false), "mod"); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.UpdateUsername(ctx, 2, 0, gin, "rightsroom"); err == nil || !strings.Contains(err.Error(), "CHAT_ADMIN_REQUIRED") {
		t.Fatalf("admin username: %v", err)
	}
	if _, err := svc.UpdateUsername(ctx, 1, 0, gin, "rightsroom"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetFullChannel(ctx, 3, gin)
	if err != nil || got.Chats[0].GetUsername().GetValue() != "rightsroom" {
		t.Fatalf("username readback %#v %v", got, err)
	}
	if got.FullChat.GetCanSetUsername() {
		t.Fatal("member can_set_username")
	}

	news, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Feed", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	bin := inCh(news.Chats[0])
	talk := megagroup(t, svc, ctx, "Talk")
	tin := inCh(talk)
	if _, err := svc.Invite(ctx, 1, 0, bin, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Invite(ctx, 1, 0, tin, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditAdmin(ctx, 1, 0, bin, 2, rights(true, false, false, false), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditAdmin(ctx, 1, 0, tin, 2, rights(false, true, false, false), ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDiscussionGroup(ctx, 2, 0, bin, tin); err == nil || !strings.Contains(err.Error(), "CHAT_ADMIN_REQUIRED") {
		t.Fatalf("discussion without linked peers: %v", err)
	}
	if _, err := svc.EditAdmin(ctx, 1, 0, bin, 2, rights(false, true, false, false), ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDiscussionGroup(ctx, 2, 0, bin, tin); err != nil {
		t.Fatal(err)
	}
	full, err := svc.GetFullChannel(ctx, 1, bin)
	if err != nil || full.FullChat.GetLinkedChatId().GetValue() != talk.Id {
		t.Fatalf("linked %#v %v", full, err)
	}
	if err := svc.SetDiscussionGroup(ctx, 2, 0, bin, mtproto.MakeTLInputChannelEmpty(&mtproto.InputChannel{}).To_InputChannel()); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.ToggleParticipantsHidden(ctx, 3, 0, gin, true); err == nil || !strings.Contains(err.Error(), "CHAT_ADMIN_REQUIRED") {
		t.Fatalf("member hide: %v", err)
	}
	if _, err := svc.ToggleParticipantsHidden(ctx, 2, 0, gin, true); err == nil || !strings.Contains(err.Error(), "CHAT_ADMIN_REQUIRED") {
		t.Fatalf("change_info hide: %v", err)
	}
	if _, err := svc.EditAdmin(ctx, 1, 0, gin, 2, rights(false, false, false, true), "mod"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ToggleParticipantsHidden(ctx, 2, 0, gin, true); err != nil {
		t.Fatal(err)
	}
	hidden, err := svc.GetParticipants(ctx, 3, gin, nil, 0, 50, 0)
	if err != nil || len(hidden.GetParticipants()) != 0 {
		t.Fatalf("hidden megagroup %#v %v", hidden, err)
	}
	full, _ = svc.GetFullChannel(ctx, 2, gin)
	if !full.FullChat.GetParticipantsHidden() {
		t.Fatal("participants_hidden not stored")
	}

	if _, err := svc.EditAdmin(ctx, 1, 0, bin, 2, rights(false, false, true, false), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdatePaidMessagesPrice(ctx, 2, 0, bin, 0, true); err != nil {
		t.Fatal(err)
	}
	full, err = svc.GetFullChannel(ctx, 1, bin)
	if err != nil || !full.Chats[0].GetBroadcastMessagesAllowed() {
		t.Fatalf("direct messages %#v %v", full, err)
	}
	if _, err := svc.EditAdmin(ctx, 1, 0, bin, 2, rights(false, false, false, false), ""); err != nil {
		t.Fatal(err)
	}
	// Empty rights demotes the admin. A plain member still cannot toggle.
	if _, err := svc.UpdatePaidMessagesPrice(ctx, 2, 0, bin, 0, false); err == nil || !strings.Contains(err.Error(), "CHAT_ADMIN_REQUIRED") {
		t.Fatalf("member paid price: %v", err)
	}

	if _, err := svc.Invite(ctx, 1, 0, bin, []int64{4}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetParticipants(ctx, 4, bin, nil, 0, 50, 0); err == nil || !strings.Contains(err.Error(), "CHAT_ADMIN_REQUIRED") {
		t.Fatalf("subscriber list: %v", err)
	}
	sub, err := svc.GetFullChannel(ctx, 4, bin)
	if err != nil || sub.FullChat.GetCanViewParticipants() {
		t.Fatalf("subscriber can_view %#v %v", sub, err)
	}
	adminList, err := svc.GetParticipants(ctx, 1, bin, nil, 0, 50, 0)
	if err != nil || adminList.GetCount() < 2 {
		t.Fatalf("admin list %#v %v", adminList, err)
	}
}
