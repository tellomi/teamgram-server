package channel

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

func TestInviteWritesAdminLogAndFiltersSplitBans(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Log")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	log, err := svc.GetAdminLog(ctx, 1, in, "", &mtproto.ChannelAdminLogEventsFilter{Invite: true}, nil, 0, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(log.GetEvents()) != 1 || log.Events[0].GetAction().GetPredicateName() != mtproto.Predicate_channelAdminLogEventActionParticipantInvite {
		t.Fatalf("invite log %#v", log.GetEvents())
	}

	kick := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{ViewMessages: true, UntilDate: 0}).To_ChatBannedRights()
	if _, err := svc.EditBanned(ctx, 1, 0, in, 2, kick); err != nil {
		t.Fatal(err)
	}
	restrict := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{SendMessages: true, UntilDate: 100}).To_ChatBannedRights()
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{3}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditBanned(ctx, 1, 0, in, 3, restrict); err != nil {
		t.Fatal(err)
	}
	kickLog, err := svc.GetAdminLog(ctx, 1, in, "", &mtproto.ChannelAdminLogEventsFilter{Kick: true}, nil, 0, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	banLog, err := svc.GetAdminLog(ctx, 1, in, "", &mtproto.ChannelAdminLogEventsFilter{Ban: true}, nil, 0, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(kickLog.GetEvents()) != 1 || len(banLog.GetEvents()) != 1 {
		t.Fatalf("kick %d ban %d", len(kickLog.GetEvents()), len(banLog.GetEvents()))
	}
	rights := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{SendMessages: true}).To_ChatBannedRights()
	if _, err := svc.EditDefaultBanned(ctx, 1, 0, in, rights); err != nil {
		t.Fatal(err)
	}
	info, err := svc.GetAdminLog(ctx, 1, in, "", &mtproto.ChannelAdminLogEventsFilter{Info: true}, nil, 0, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range info.GetEvents() {
		if ev.GetAction().GetPredicateName() == mtproto.Predicate_channelAdminLogEventActionDefaultBannedRights {
			t.Fatal("default rights showed up under info")
		}
	}
	settings, err := svc.GetAdminLog(ctx, 1, in, "", &mtproto.ChannelAdminLogEventsFilter{Settings: true}, nil, 0, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	var saw bool
	for _, ev := range settings.GetEvents() {
		if ev.GetAction().GetPredicateName() == mtproto.Predicate_channelAdminLogEventActionDefaultBannedRights {
			saw = true
		}
	}
	if !saw {
		t.Fatal("default rights missing from settings")
	}
}

func TestDiscussionClearsHiddenPrehistory(t *testing.T) {
	svc, ctx := newTest(t)
	news, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "news", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	group := megagroup(t, svc, ctx, "talk")
	gin := inCh(group)
	if _, err := svc.TogglePreHistory(ctx, 1, 0, gin, true); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDiscussionGroup(ctx, 1, 0, inCh(news.Chats[0]), gin); err != nil {
		t.Fatal(err)
	}
	full, err := svc.GetFullChannel(ctx, 1, gin)
	if err != nil || full.GetFullChat().GetHiddenPrehistory() {
		t.Fatalf("still hidden %#v %v", full.GetFullChat().GetHiddenPrehistory(), err)
	}
	if _, err := svc.TogglePreHistory(ctx, 1, 0, gin, true); status.Convert(err).Message() != "CHAT_LINK_EXISTS" {
		t.Fatalf("hide linked group: %v", err)
	}
}

func TestDeclineJoinPushesTheApplicant(t *testing.T) {
	svc, ctx := newTest(t)
	var got int64
	svc.SetHooks(Hooks{Push: func(_ context.Context, userID int64, _ []int64, _ *mtproto.Updates) {
		got = userID
	}})
	ch := megagroup(t, svc, ctx, "door")
	in := inCh(ch)
	if err := svc.recordJoinRequest(ctx, ch.Id, 9); err != nil {
		t.Fatal(err)
	}
	if err := svc.ApproveJoin(ctx, 1, 0, in, 9, false); err != nil {
		t.Fatal(err)
	}
	if got != 9 {
		t.Fatalf("declined user %d", got)
	}
}
