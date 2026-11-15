package channel

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

func TestMegagroupParticipantRank(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Ranks", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}

	if _, err = svc.EditParticipantRank(ctx, 2, ch.Id, 2, "tag"); err != nil {
		t.Fatal(err)
	}
	if got := participantRank(t, svc, ctx, in, 2); got != "tag" {
		t.Fatalf("self rank %q", got)
	}
	if _, err = svc.EditParticipantRank(ctx, 2, ch.Id, 3, "nope"); status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("member edited someone else: %v", err)
	}

	rights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{ManageRanks: true}).To_ChatAdminRights()
	if _, err = svc.EditAdmin(ctx, 1, 0, in, 2, rights, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.EditParticipantRank(ctx, 2, ch.Id, 3, "mod"); err != nil {
		t.Fatal(err)
	}
	if got := participantRank(t, svc, ctx, in, 3); got != "mod" {
		t.Fatalf("admin rank %q", got)
	}

	ban := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{EditRank: true}).To_ChatBannedRights()
	if _, err = svc.EditDefaultBanned(ctx, 1, 0, in, ban); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.EditParticipantRank(ctx, 3, ch.Id, 3, "own"); status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("banned self edit: %v", err)
	}
	if _, err = svc.EditParticipantRank(ctx, 2, ch.Id, 3, "kept"); err != nil {
		t.Fatalf("manage_ranks admin: %v", err)
	}

	if _, err = svc.EditParticipantRank(ctx, 1, ch.Id, 3, "this rank is too long"); status.Convert(err).Message() != "ADMIN_RANK_INVALID" {
		t.Fatalf("long rank: %v", err)
	}

	filter := mtproto.MakeTLChannelAdminLogEventsFilter(&mtproto.ChannelAdminLogEventsFilter{EditRank: true}).To_ChannelAdminLogEventsFilter()
	log, err := svc.GetAdminLog(ctx, 1, in, "", filter, nil, 0, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	var saw bool
	for _, ev := range log.GetEvents() {
		act := ev.GetAction()
		if act.GetPredicateName() != mtproto.Predicate_channelAdminLogEventActionParticipantEditRank {
			t.Fatalf("filter returned %s", act.GetPredicateName())
		}
		if act.GetUserId() == 3 && act.GetNewRank() == "kept" && act.GetPrevRank() == "mod" {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("edit_rank log missing, events %d", len(log.GetEvents()))
	}
}

func participantRank(t *testing.T, svc *Service, ctx context.Context, in *mtproto.InputChannel, userID int64) string {
	t.Helper()
	got, err := svc.GetParticipant(ctx, 1, in, mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: userID}).To_InputPeer())
	if err != nil {
		t.Fatal(err)
	}
	return got.GetParticipant().GetRank().GetValue()
}
