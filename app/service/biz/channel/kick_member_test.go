package channel

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestKickRemovesMemberAndUnbanDoesNotReadd(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}

	// Clients send view_messages with until_date 0 for a permanent kick.
	kick := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{
		ViewMessages: true,
	}).To_ChatBannedRights()
	if _, err := svc.EditBanned(ctx, 1, 0, in, 2, kick); err != nil {
		t.Fatal(err)
	}
	if userInParticipants(t, svc, ctx, in, nil, 2) {
		t.Fatal("kicked user still listed in recent participants")
	}
	kickedFilter := mtproto.MakeTLChannelParticipantsKicked(&mtproto.ChannelParticipantsFilter{}).To_ChannelParticipantsFilter()
	if !userInParticipants(t, svc, ctx, in, kickedFilter, 2) {
		t.Fatal("kicked user missing from the kicked list")
	}
	if _, err := svc.Join(ctx, 2, 0, in); err == nil {
		t.Fatal("kicked user rejoined")
	}

	open := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{}).To_ChatBannedRights()
	if _, err := svc.EditBanned(ctx, 1, 0, in, 2, open); err != nil {
		t.Fatal(err)
	}
	if userInParticipants(t, svc, ctx, in, nil, 2) {
		t.Fatal("unban put the user back in the member list")
	}
	if userInParticipants(t, svc, ctx, in, kickedFilter, 2) {
		t.Fatal("unban left the user on the kicked list")
	}
	// A private group still needs an invite. Unban only makes that invite succeed.
	invited, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, miss := range invited.GetMissingInvitees() {
		if miss.GetUserId() == 2 {
			t.Fatal("unban left the user blocked from invites")
		}
	}
	if !userInParticipants(t, svc, ctx, in, nil, 2) {
		t.Fatal("invite after unban did not add the user")
	}
}

func TestUnbanOfStoredViewBanDoesNotReadd(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	// A kick saved before removal kept StateNormal plus the view_messages bit.
	if err := svc.store.PutParticipant(ctx, &Participant{
		ChannelID:    ch.Id,
		UserID:       3,
		Role:         RoleBanned,
		State:        StateNormal,
		BannedRights: int64(mtproto.BANNED_VIEW_MESSAGES),
		Date:         1,
	}); err != nil {
		t.Fatal(err)
	}
	if !userInParticipants(t, svc, ctx, in, nil, 3) {
		t.Fatal("fixture user missing before unban")
	}
	open := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{}).To_ChatBannedRights()
	if _, err := svc.EditBanned(ctx, 1, 0, in, 3, open); err != nil {
		t.Fatal(err)
	}
	if userInParticipants(t, svc, ctx, in, nil, 3) {
		t.Fatal("unban re-added a user who had been kicked with view_messages")
	}
}

func TestSendRestrictionDoesNotRemoveMember(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	mute := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{
		SendMessages: true,
		UntilDate:    int32(1<<31 - 1),
	}).To_ChatBannedRights()
	if _, err := svc.EditBanned(ctx, 1, 0, in, 2, mute); err != nil {
		t.Fatal(err)
	}
	if !userInParticipants(t, svc, ctx, in, nil, 2) {
		t.Fatal("a send restriction removed the member")
	}
	open := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{}).To_ChatBannedRights()
	if _, err := svc.EditBanned(ctx, 1, 0, in, 2, open); err != nil {
		t.Fatal(err)
	}
	if !userInParticipants(t, svc, ctx, in, nil, 2) {
		t.Fatal("lifting a send restriction removed the member")
	}
}

func userInParticipants(t *testing.T, svc *Service, ctx context.Context, in *mtproto.InputChannel, filter *mtproto.ChannelParticipantsFilter, userID int64) bool {
	t.Helper()
	got, err := svc.GetParticipants(ctx, 1, in, filter, 0, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range got.GetParticipants() {
		if participantUserID(p) == userID {
			return true
		}
	}
	return false
}

func participantUserID(p *mtproto.ChannelParticipant) int64 {
	if p == nil {
		return 0
	}
	if p.GetUserId() != 0 {
		return p.GetUserId()
	}
	if p.GetPeer() != nil {
		return p.GetPeer().GetUserId()
	}
	return 0
}
