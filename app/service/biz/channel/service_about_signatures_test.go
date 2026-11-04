package channel

import (
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

func TestEditAboutAndAdminLog(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true, About: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditAbout(ctx, 2, 0, in, "nope"); err != mtproto.ErrChatAdminRequired {
		t.Fatalf("member edit: %v", err)
	}
	if _, err := svc.EditAbout(ctx, 1, 0, in, strings.Repeat("a", 256)); err != mtproto.ErrChatAboutTooLong {
		t.Fatalf("too long: %v", err)
	}
	if _, err := svc.EditAbout(ctx, 1, 0, in, "hello"); err != mtproto.ErrChatAboutNotModified {
		t.Fatalf("same text: %v", err)
	}
	if _, err := svc.EditAbout(ctx, 1, 0, in, "updated"); err != nil {
		t.Fatal(err)
	}
	full, err := svc.GetFullChannel(ctx, 1, in)
	if err != nil || full.FullChat.GetAbout() != "updated" {
		t.Fatalf("about %q %v", full.GetFullChat().GetAbout(), err)
	}
	if _, err := svc.EditAbout(ctx, 1, 0, in, ""); err != nil {
		t.Fatal(err)
	}
	full, _ = svc.GetFullChannel(ctx, 1, in)
	if full.FullChat.GetAbout() != "" {
		t.Fatalf("cleared about %q", full.FullChat.GetAbout())
	}

	log, err := svc.GetAdminLog(ctx, 1, in, "", nil, nil, 0, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	var sawAbout bool
	for _, ev := range log.GetEvents() {
		if ev.GetAction().GetPredicateName() == mtproto.Predicate_channelAdminLogEventActionChangeAbout &&
			ev.GetAction().GetNewValue_STRING() == "updated" &&
			ev.GetAction().GetPrevValue_STRING() == "hello" {
			sawAbout = true
		}
	}
	if !sawAbout {
		t.Fatalf("admin log missing changeAbout: %#v", log.GetEvents())
	}
}

func TestToggleSignaturesFlagsAndLog(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.ToggleSignatures(ctx, 1, 0, in, false, false); status.Convert(err).Message() != "CHAT_NOT_MODIFIED" {
		t.Fatalf("unchanged: %v", err)
	}
	if _, err := svc.ToggleSignatures(ctx, 1, 0, in, true, true); err != nil {
		t.Fatal(err)
	}
	visible := svc.ChatFor(ctx, 1, mustChannel(t, svc, ctx, ch.Id))
	if !visible.GetSignatures() || !visible.GetSignatureProfiles() {
		t.Fatalf("flags signatures=%v profiles=%v", visible.GetSignatures(), visible.GetSignatureProfiles())
	}
	if _, err := svc.ToggleSignatures(ctx, 1, 0, in, true, false); err != nil {
		t.Fatal(err)
	}
	visible = svc.ChatFor(ctx, 1, mustChannel(t, svc, ctx, ch.Id))
	if !visible.GetSignatures() || visible.GetSignatureProfiles() {
		t.Fatalf("after profiles off signatures=%v profiles=%v", visible.GetSignatures(), visible.GetSignatureProfiles())
	}
	log, err := svc.GetAdminLog(ctx, 1, in, "", nil, nil, 0, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	var sig, prof bool
	for _, ev := range log.GetEvents() {
		switch ev.GetAction().GetPredicateName() {
		case mtproto.Predicate_channelAdminLogEventActionToggleSignatures:
			sig = mtproto.FromBool(ev.GetAction().GetNewValue_BOOL())
		case mtproto.Predicate_channelAdminLogEventActionToggleSignatureProfiles:
			if mtproto.FromBool(ev.GetAction().GetNewValue_BOOL()) {
				prof = true
			}
		}
	}
	if !sig || !prof {
		t.Fatalf("admin log signatures=%v profiles=%v events=%d", sig, prof, len(log.GetEvents()))
	}
}

func TestPromoteAndBanWriteAdminLog(t *testing.T) {
	svc, ctx := newTest(t)
	created, _ := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Mods", Megagroup: true})
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	rights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{DeleteMessages: true}).To_ChatAdminRights()
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 2, rights, "mod"); err != nil {
		t.Fatal(err)
	}
	ban := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{SendMessages: true}).To_ChatBannedRights()
	if _, err := svc.EditBanned(ctx, 1, 0, in, 2, ban); err != nil {
		t.Fatal(err)
	}
	log, err := svc.GetAdminLog(ctx, 1, in, "", nil, nil, 0, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	var promote, banned bool
	for _, ev := range log.GetEvents() {
		switch ev.GetAction().GetPredicateName() {
		case mtproto.Predicate_channelAdminLogEventActionParticipantToggleAdmin:
			promote = true
		case mtproto.Predicate_channelAdminLogEventActionParticipantToggleBan:
			banned = true
		}
	}
	if !promote || !banned {
		t.Fatalf("promote=%v ban=%v", promote, banned)
	}
}
