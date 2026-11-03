package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestPickSuccessorPrefersEarliestAdmin(t *testing.T) {
	parts := []*Participant{
		{UserID: 1, Role: RoleCreator, State: StateNormal, Date: 1},
		{UserID: 2, Role: RoleMember, State: StateNormal, Date: 2},
		{UserID: 3, Role: RoleAdmin, State: StateNormal, Date: 50},
		{UserID: 4, Role: RoleAdmin, State: StateNormal, Date: 40},
		{UserID: 5, Role: RoleMember, State: StateNormal, Date: 3},
		{UserID: 6, Role: RoleBanned, State: StateNormal, Date: 1},
		{UserID: 7, Role: RoleAdmin, State: StateLeft, Date: 1},
	}
	got, ok := PickSuccessor(parts, 1)
	if !ok || got != 4 {
		t.Fatalf("successor = %d ok=%v, want admin 4", got, ok)
	}
	got, ok = PickSuccessor([]*Participant{
		{UserID: 1, Role: RoleCreator, State: StateNormal},
		{UserID: 9, Role: RoleMember, State: StateNormal, Date: 8},
		{UserID: 8, Role: RoleMember, State: StateNormal, Date: 8},
	}, 1)
	if !ok || got != 8 {
		t.Fatalf("member successor = %d ok=%v, want 8", got, ok)
	}
	if _, ok = PickSuccessor([]*Participant{{UserID: 1, Role: RoleCreator, State: StateNormal}}, 1); ok {
		t.Fatal("sole member should not transfer")
	}
}

func TestLeaveDeletedAccountTransfersThenLeaves(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	rights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{BanUsers: true, InviteUsers: true}).To_ChatAdminRights()
	if _, err = svc.EditAdmin(ctx, 1, 0, in, 3, rights, "mod"); err != nil {
		t.Fatal(err)
	}
	// 2 joined first. 3 is the only admin, so 3 becomes creator.
	for _, uid := range []int64{2, 3} {
		p, err := svc.store.GetParticipant(ctx, ch.Id, uid)
		if err != nil || p == nil {
			t.Fatal(err)
		}
		p.Date = int32(uid)
		if err = svc.store.PutParticipant(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	before, err := svc.store.ListMessages(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) == 0 {
		t.Fatal("create should have stored a service message")
	}
	if err = svc.LeaveDeletedAccount(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err = svc.LeaveDeletedAccount(ctx, 1); err != nil {
		t.Fatal("retry")
	}
	got, err := svc.store.GetChannel(ctx, ch.Id)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.Deleted || got.CreatorID != 3 {
		t.Fatalf("channel deleted=%v creator=%d", got.Deleted, got.CreatorID)
	}
	left, err := svc.store.GetParticipant(ctx, ch.Id, 1)
	if err != nil || left == nil || left.State != StateLeft {
		t.Fatalf("leaver = %#v %v", left, err)
	}
	creator, err := svc.store.GetParticipant(ctx, ch.Id, 3)
	if err != nil || creator == nil || creator.Role != RoleCreator {
		t.Fatalf("new creator = %#v", creator)
	}
	after, err := svc.store.ListMessages(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) < len(before) {
		t.Fatalf("messages %d -> %d", len(before), len(after))
	}
	if got.ParticipantsCount != 2 {
		t.Fatalf("count = %d", got.ParticipantsCount)
	}
}

func TestLeaveDeletedAccountDeactivatesSoleMember(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 7, Title: "Solo", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	chID := created.Chats[0].Id
	before, err := svc.store.ListMessages(ctx, chID)
	if err != nil || len(before) == 0 {
		t.Fatalf("messages %d %v", len(before), err)
	}
	if err = svc.LeaveDeletedAccount(ctx, 7); err != nil {
		t.Fatal(err)
	}
	got, err := svc.store.GetChannel(ctx, chID)
	if err != nil || got == nil || !got.Deleted {
		t.Fatalf("channel = %#v %v", got, err)
	}
	if got.ParticipantsCount != 0 {
		t.Fatalf("count = %d", got.ParticipantsCount)
	}
	after, err := svc.store.ListMessages(ctx, chID)
	if err != nil || len(after) < len(before) {
		t.Fatalf("messages kept %d -> %d (%v)", len(before), len(after), err)
	}
}
