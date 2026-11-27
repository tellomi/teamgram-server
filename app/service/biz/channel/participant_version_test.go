package channel

import (
	"context"
	"errors"
	"testing"
)

func TestRowVersionIgnoresCallerAndBlocksUnobservedOverwrite(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Version")
	fresh := &Participant{ChannelID: ch.Id, UserID: 21, Role: RoleMember, State: StateNormal, Date: 10, rowVersion: 9}
	if err := svc.store.PutParticipant(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	got, err := svc.store.GetParticipant(ctx, ch.Id, 21)
	if err != nil || got == nil || got.rowVersion != 1 {
		t.Fatalf("insert version %+v %v", got, err)
	}
	if err = svc.store.PutParticipant(ctx, nil); err == nil {
		t.Fatal("nil participant write succeeded")
	}
	again, err := svc.store.GetParticipant(ctx, ch.Id, 21)
	if err != nil || again == nil || again.Date != 10 || again.State != StateNormal || again.rowVersion != 1 {
		t.Fatalf("nil write changed the row %+v %v", again, err)
	}
	clobber := &Participant{ChannelID: ch.Id, UserID: 21, Role: RoleMember, State: StateLeft, Date: 1, rowVersion: 9}
	if err = svc.store.PutParticipant(ctx, clobber); !errors.Is(err, errParticipantChanged) {
		t.Fatalf("unobserved overwrite err=%v", err)
	}
	got, err = svc.store.GetParticipant(ctx, ch.Id, 21)
	if err != nil || got.State != StateNormal || got.Date != 10 || got.rowVersion != 1 {
		t.Fatalf("unobserved overwrite changed %+v %v", got, err)
	}
	got.Rank = "kept"
	if err = svc.store.PutParticipant(ctx, got); err != nil {
		t.Fatal(err)
	}
	got, err = svc.store.GetParticipant(ctx, ch.Id, 21)
	if err != nil || got.rowVersion != 2 || got.Rank != "kept" {
		t.Fatalf("observed update %+v %v", got, err)
	}
}

func TestMigrateReportsWriteErrorsAndDoesNotClobber(t *testing.T) {
	svc, ctx := newTest(t)
	up, err := svc.Migrate(ctx, MigrateRequest{
		Actor: 1, ActorAuth: 1, CreatorID: 1, ChatID: 501, Title: "Moved",
		Members: []LegacyMember{{UserID: 21}, {UserID: 22}},
	})
	if err != nil || up == nil {
		t.Fatal(err)
	}
	for _, id := range []int64{1, 21, 22} {
		p, err := svc.store.GetParticipant(ctx, up.ChannelID, id)
		if err != nil || p == nil || !p.InChannel() || p.rowVersion != 1 {
			t.Fatalf("imported %d %+v %v", id, p, err)
		}
	}
	kept, err := svc.store.GetParticipant(ctx, up.ChannelID, 21)
	if err != nil {
		t.Fatal(err)
	}
	kept.Date = 42
	kept.Rank = "rejoined"
	if err = svc.store.PutParticipant(ctx, kept); err != nil {
		t.Fatal(err)
	}
	again, err := svc.Migrate(ctx, MigrateRequest{
		Actor: 1, ActorAuth: 1, CreatorID: 1, ChatID: 501, Title: "Moved",
		Members: []LegacyMember{{UserID: 21}},
	})
	if err != nil || again == nil || !again.Already {
		t.Fatalf("second migrate %+v %v", again, err)
	}
	// A new channel is not created. The existing member is not rewritten
	// because FindByMigratedChat returns the first channel. Seed a second
	// import on a fresh chat whose member row was inserted first.
	seeded, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Seed", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	prior := &Participant{ChannelID: seeded.Chats[0].Id, UserID: 21, Role: RoleAdmin, State: StateNormal, Date: 7, Rank: "stay"}
	if err = svc.store.PutParticipant(ctx, prior); err != nil {
		t.Fatal(err)
	}
	before, err := svc.store.GetParticipant(ctx, seeded.Chats[0].Id, 21)
	if err != nil {
		t.Fatal(err)
	}
	err = svc.store.PutParticipant(ctx, &Participant{
		ChannelID: seeded.Chats[0].Id, UserID: 21, Role: RoleMember, State: StateNormal, Date: 99,
	})
	if !errors.Is(err, errParticipantChanged) {
		t.Fatalf("import overwrite err=%v", err)
	}
	after, err := svc.store.GetParticipant(ctx, seeded.Chats[0].Id, 21)
	if err != nil || after.Role != RoleAdmin || after.Date != 7 || after.Rank != "stay" || after.rowVersion != before.rowVersion {
		t.Fatalf("import overwrite changed %+v %v", after, err)
	}
	base := svc.store
	svc.store = &failOneParticipant{Store: base, user: 32}
	_, err = svc.Migrate(ctx, MigrateRequest{
		Actor: 1, ActorAuth: 1, CreatorID: 1, ChatID: 502, Title: "Partial",
		Members: []LegacyMember{{UserID: 31}, {UserID: 32}},
	})
	svc.store = base
	if err == nil || !errors.Is(err, errParticipantWrite) {
		t.Fatalf("migrate hid a write error: %v", err)
	}
}

var errParticipantWrite = errors.New("participant write failed")

type failOneParticipant struct {
	Store
	user int64
}

func (f *failOneParticipant) PutParticipant(ctx context.Context, p *Participant) error {
	if p != nil && p.UserID == f.user {
		return errParticipantWrite
	}
	return f.Store.PutParticipant(ctx, p)
}
