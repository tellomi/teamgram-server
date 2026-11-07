package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestStatusRecipientsPermissionAndIdempotent(t *testing.T) {
	blocked := map[int64]bool{4: true}
	contacts := []int64{2, 1, 0, 2, 3, 4, 3}
	first := statusRecipients(1, contacts, func(id int64) bool { return blocked[id] })
	second := statusRecipients(1, contacts, func(id int64) bool { return blocked[id] })
	if len(first) != 2 || first[0] != 2 || first[1] != 3 {
		t.Fatalf("recipients %v", first)
	}
	if len(second) != len(first) || second[0] != first[0] || second[1] != first[1] {
		t.Fatalf("repeat %v want %v", second, first)
	}
}

func TestDeliverStatusReachesOtherSessions(t *testing.T) {
	updates := mtproto.MakeTLUpdateShort(&mtproto.Updates{
		Update: mtproto.MakeTLUpdateUserStatus(&mtproto.Update{UserId: 1}).To_Update(),
	}).To_Updates()
	var contacts []int64
	var other int
	push := func(id int64, _ *mtproto.Updates) { contacts = append(contacts, id) }
	sessions := func(self, auth int64, _ *mtproto.Updates) {
		if self != 1 || auth != 9 {
			t.Fatalf("session %d %d", self, auth)
		}
		other++
	}
	deliverUserStatus(1, 9, updates, []int64{2}, func(int64) bool { return false }, push, sessions)
	deliverUserStatus(1, 9, updates, []int64{2}, func(int64) bool { return false }, push, sessions)
	if len(contacts) != 2 || contacts[0] != 2 || contacts[1] != 2 || other != 2 {
		t.Fatalf("contacts %v other %d", contacts, other)
	}
	other = 0
	deliverUserStatus(1, 0, updates, []int64{2}, nil, func(int64, *mtproto.Updates) {}, sessions)
	if other != 0 {
		t.Fatal("missing auth key still pushed to other sessions")
	}
}
