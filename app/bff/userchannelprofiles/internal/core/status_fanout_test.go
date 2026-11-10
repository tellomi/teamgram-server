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

func TestDeliverStatusReachesContactsOnly(t *testing.T) {
	updates := mtproto.MakeTLUpdateShort(&mtproto.Updates{
		Update: mtproto.MakeTLUpdateUserStatus(&mtproto.Update{UserId: 1}).To_Update(),
	}).To_Updates()
	var contacts []int64
	push := func(id int64, _ *mtproto.Updates) { contacts = append(contacts, id) }
	deliverUserStatus(1, updates, []int64{2, 1, 2}, func(int64) bool { return false }, push)
	deliverUserStatus(1, updates, []int64{2, 1, 2}, func(int64) bool { return false }, push)
	if len(contacts) != 2 || contacts[0] != 2 || contacts[1] != 2 {
		t.Fatalf("contacts %v", contacts)
	}
	deliverUserStatus(1, updates, []int64{2}, nil, nil)
}
