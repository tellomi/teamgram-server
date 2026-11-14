package channel

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestExpiredMuteIsUnrestricted(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	now := int32(time.Now().Unix())
	live := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{
		SendMessages: true,
		UntilDate:    now + 3600,
	}).To_ChatBannedRights()
	if _, err = svc.EditBanned(ctx, 1, 0, in, 2, live); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{
		RandomID: 21,
		Message:  &mtproto.Message{Message: "muted", Date: now},
	}}); err == nil {
		t.Fatal("live mute still accepted a message")
	}
	full, err := svc.GetFullChannel(ctx, 2, in)
	if err != nil {
		t.Fatal(err)
	}
	if br := viewerBanned(full); br == nil || !br.GetSendMessages() || br.GetUntilDate() != now+3600 {
		t.Fatalf("live banned_rights = %#v", br)
	}
	if got := participantOf(t, svc, ctx, in, 2); got.GetPredicateName() != mtproto.Predicate_channelParticipantBanned {
		t.Fatalf("live participant = %s", got.GetPredicateName())
	}

	expired := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{
		SendMessages: true,
		UntilDate:    now - 30,
	}).To_ChatBannedRights()
	if _, err = svc.EditBanned(ctx, 1, 0, in, 2, expired); err != nil {
		t.Fatal(err)
	}
	full, err = svc.GetFullChannel(ctx, 2, in)
	if err != nil {
		t.Fatal(err)
	}
	if br := viewerBanned(full); br != nil {
		t.Fatalf("expired banned_rights still sent: %#v", br)
	}
	if got := participantOf(t, svc, ctx, in, 2); got.GetPredicateName() == mtproto.Predicate_channelParticipantBanned || got.GetBannedRights() != nil {
		t.Fatalf("expired participant = %s rights %#v", got.GetPredicateName(), got.GetBannedRights())
	}
	bannedFilter := mtproto.MakeTLChannelParticipantsBanned(&mtproto.ChannelParticipantsFilter{}).To_ChannelParticipantsFilter()
	if userInParticipants(t, svc, ctx, in, bannedFilter, 2) {
		t.Fatal("expired mute stayed on the banned list")
	}
	if !userInParticipants(t, svc, ctx, in, nil, 2) {
		t.Fatal("expired mute removed the member")
	}
	if _, err = svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{
		RandomID: 22,
		Message:  &mtproto.Message{Message: "open", Date: now},
	}}); err != nil {
		t.Fatalf("send after expiry: %v", err)
	}

	pastDefault := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{
		SendMessages: true,
		UntilDate:    now - 10,
	}).To_ChatBannedRights()
	if _, err = svc.EditDefaultBanned(ctx, 1, 0, in, pastDefault); err != nil {
		t.Fatal(err)
	}
	full, err = svc.GetFullChannel(ctx, 1, in)
	if err != nil {
		t.Fatal(err)
	}
	if br := full.Chats[0].GetDefaultBannedRights(); br != nil {
		t.Fatalf("expired default_banned_rights = %#v", br)
	}
}

func viewerBanned(full *mtproto.Messages_ChatFull) *mtproto.ChatBannedRights {
	if full == nil || len(full.Chats) == 0 {
		return nil
	}
	return full.Chats[0].GetBannedRights()
}

func participantOf(t *testing.T, svc *Service, ctx context.Context, in *mtproto.InputChannel, userID int64) *mtproto.ChannelParticipant {
	t.Helper()
	got, err := svc.GetParticipant(ctx, 1, in, mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: userID}).To_InputPeer())
	if err != nil {
		t.Fatal(err)
	}
	return got.GetParticipant()
}
