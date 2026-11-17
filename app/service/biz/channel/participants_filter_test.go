package channel

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestParticipantNameContactBanAndHiddenAdmins(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Filters")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2, 3, 4}, nil); err != nil {
		t.Fatal(err)
	}
	svc.SetHooks(Hooks{UserCards: func(_ context.Context, _ int64, _ []int64) map[int64]UserCard {
		return map[int64]UserCard{
			// ToSelfUser marks the viewer as a contact. The contacts
			// filter must still leave them out.
			1: {First: "Owner", Contact: true},
			2: {First: "Ada", Username: "ada", Contact: true},
			3: {First: "Bob", Contact: false},
			4: {First: "Cara", Last: "Lee", Contact: true},
		}
	}})

	contacts := mtproto.MakeTLChannelParticipantsContacts(&mtproto.ChannelParticipantsFilter{}).To_ChannelParticipantsFilter()
	got, err := svc.GetParticipants(ctx, 1, in, contacts, 0, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ids := participantIDs(got); !sameIDs(ids, []int64{2, 4}) || containsID(ids, 1) {
		t.Fatalf("contacts %v, viewer 1 must be absent", ids)
	}

	// q searches inside the contact set and still drops the viewer,
	// even when the viewer's own name matches.
	contactsQ := mtproto.MakeTLChannelParticipantsContacts(&mtproto.ChannelParticipantsFilter{Q_STRING: "a"}).To_ChannelParticipantsFilter()
	got, err = svc.GetParticipants(ctx, 1, in, contactsQ, 0, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ids := participantIDs(got); !sameIDs(ids, []int64{2, 4}) || containsID(ids, 1) {
		t.Fatalf("contacts q %v", ids)
	}
	selfOnly := mtproto.MakeTLChannelParticipantsContacts(&mtproto.ChannelParticipantsFilter{Q_STRING: "owner"}).To_ChannelParticipantsFilter()
	got, err = svc.GetParticipants(ctx, 1, in, selfOnly, 0, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ids := participantIDs(got); len(ids) != 0 || got.Count != 0 {
		t.Fatalf("contacts q matching only the viewer: %v count %d", ids, got.Count)
	}

	search := mtproto.MakeTLChannelParticipantsSearch(&mtproto.ChannelParticipantsFilter{Q_STRING: "ADA"}).To_ChannelParticipantsFilter()
	got, err = svc.GetParticipants(ctx, 1, in, search, 0, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ids := participantIDs(got); !sameIDs(ids, []int64{2}) || got.Count != 1 {
		t.Fatalf("search %v count %d", ids, got.Count)
	}

	mentions := mtproto.MakeTLChannelParticipantsMentions(&mtproto.ChannelParticipantsFilter{
		Q_FLAGSTRING: &wrapperspb.StringValue{Value: "lee"},
	}).To_ChannelParticipantsFilter()
	got, err = svc.GetParticipants(ctx, 1, in, mentions, 0, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ids := participantIDs(got); !sameIDs(ids, []int64{4}) {
		t.Fatalf("mentions %v", ids)
	}

	if _, err := svc.EditAdmin(ctx, 1, 0, in, 2, mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{
		ChangeInfo: true,
	}).To_ChatAdminRights(), ""); err != nil {
		t.Fatal(err)
	}
	kick := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{ViewMessages: true}).To_ChatBannedRights()
	if _, err := svc.EditBanned(ctx, 1, 0, in, 3, kick); err != nil {
		t.Fatal(err)
	}
	kicked := mtproto.MakeTLChannelParticipantsKicked(&mtproto.ChannelParticipantsFilter{Q_STRING: "bob"}).To_ChannelParticipantsFilter()
	if _, err := svc.GetParticipants(ctx, 4, in, kicked, 0, 50, 0); status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("member kicked list: %v", err)
	}
	if _, err := svc.GetParticipants(ctx, 2, in, kicked, 0, 50, 0); status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("admin without ban_users: %v", err)
	}
	got, err = svc.GetParticipants(ctx, 1, in, kicked, 0, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ids := participantIDs(got); !sameIDs(ids, []int64{3}) {
		t.Fatalf("kicked search %v", ids)
	}
	banned := mtproto.MakeTLChannelParticipantsBanned(&mtproto.ChannelParticipantsFilter{Q_STRING: "nobody"}).To_ChannelParticipantsFilter()
	got, err = svc.GetParticipants(ctx, 1, in, banned, 0, 50, 0)
	if err != nil || got.Count != 0 {
		t.Fatalf("banned miss count=%d err=%v", got.GetCount(), err)
	}

	if _, err := svc.ToggleParticipantsHidden(ctx, 1, 0, in, true); err != nil {
		t.Fatal(err)
	}
	hidden, err := svc.GetParticipants(ctx, 4, in, nil, 0, 50, 0)
	if err != nil || len(hidden.Participants) != 0 {
		t.Fatalf("hidden recent %#v %v", hidden, err)
	}
	admins := mtproto.MakeTLChannelParticipantsAdmins(&mtproto.ChannelParticipantsFilter{}).To_ChannelParticipantsFilter()
	got, err = svc.GetParticipants(ctx, 4, in, admins, 0, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ids := participantIDs(got); !sameIDs(ids, []int64{1, 2}) {
		t.Fatalf("hidden admins %v", ids)
	}
	full, err := svc.GetFullChannel(ctx, 4, in)
	if err != nil || full.FullChat.GetCanViewParticipants() || !full.FullChat.GetParticipantsHidden() {
		t.Fatalf("member full can_view=%v hidden=%v err=%v", full.GetFullChat().GetCanViewParticipants(), full.GetFullChat().GetParticipantsHidden(), err)
	}
	full, err = svc.GetFullChannel(ctx, 1, in)
	if err != nil || !full.FullChat.GetCanViewParticipants() {
		t.Fatalf("creator can_view=%v err=%v", full.GetFullChat().GetCanViewParticipants(), err)
	}
}

func participantIDs(got *mtproto.Channels_ChannelParticipants) []int64 {
	out := make([]int64, 0, len(got.GetParticipants()))
	for _, p := range got.GetParticipants() {
		id := p.GetUserId()
		if id == 0 && p.GetPeer() != nil {
			id = p.GetPeer().GetUserId()
		}
		out = append(out, id)
	}
	return out
}

func sameIDs(got, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[int64]int{}
	for _, id := range got {
		seen[id]++
	}
	for _, id := range want {
		seen[id]--
		if seen[id] < 0 {
			return false
		}
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}
