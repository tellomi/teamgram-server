package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestMigrateKeepsDefaultBannedRights(t *testing.T) {
	svc, ctx := newTest(t)
	banned := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{
		SendMedia:  true,
		SendPhotos: true,
		UntilDate:  0,
	}).To_ChatBannedRights()
	migrated, err := svc.Migrate(ctx, MigrateRequest{
		Actor:     1,
		ChatID:    77,
		Title:     "OldRoom",
		CreatorID: 1,
		Members:   []LegacyMember{{UserID: 2}},
		Banned:    banned,
	})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := svc.store.GetChannel(ctx, migrated.ChannelID)
	if err != nil {
		t.Fatal(err)
	}
	rights := mtproto.BannedRights(ch.DefaultBanned).ToChatBannedRights()
	if !rights.GetSendMedia() || !rights.GetSendPhotos() {
		t.Fatalf("default banned = %#v", rights)
	}
}

func TestLegacyInvitePreviewsAndJoins(t *testing.T) {
	svc, ctx := newTest(t)
	migrated, err := svc.Migrate(ctx, MigrateRequest{
		Actor:     1,
		ChatID:    88,
		Title:     "Moved",
		CreatorID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	in := input(migrated.ChannelID, migrated.AccessHash)
	preview, err := svc.LegacyInvitePreview(ctx, 4, in)
	if err != nil {
		t.Fatal(err)
	}
	if preview.GetPredicateName() != mtproto.Predicate_chatInvite || !preview.GetMegagroup() || preview.GetTitle() != "Moved" {
		t.Fatalf("preview %#v", preview)
	}
	if _, err = svc.JoinFromLegacyInvite(ctx, 4, 0, in); err != nil {
		t.Fatal(err)
	}
	me, err := svc.GetParticipant(ctx, 4, in, mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer())
	if err != nil {
		t.Fatal(err)
	}
	if me.GetParticipant().GetUserId() != 4 {
		t.Fatalf("joined %#v", me.GetParticipant())
	}
	again, err := svc.LegacyInvitePreview(ctx, 4, in)
	if err != nil || again.GetPredicateName() != mtproto.Predicate_chatInviteAlready {
		t.Fatalf("already %#v %v", again, err)
	}
}
