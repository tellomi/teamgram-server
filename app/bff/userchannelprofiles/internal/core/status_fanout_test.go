package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestStatusRecipientsFailClosedWithoutPrivacy(t *testing.T) {
	if got := statusRecipients(1, []int64{2}, []int64{5}, nil, nil); len(got) != 0 {
		t.Fatalf("missing privacy must not fan out, got %v", got)
	}
	if got := statusDeliveries(1, mtproto.MakeUserStatusOnline(), 10, mtproto.UserOnlinePeriod, 10, []int64{2}, []int64{5}, nil, nil); len(got) != 0 {
		t.Fatalf("missing privacy deliveries %v", got)
	}
}

func TestStatusDeliveryReverseRule(t *testing.T) {
	const (
		self = int64(1)
		seen = int64(1_700_000_000)
	)
	now := seen + 20
	exact := mtproto.PresenceStatus(seen, mtproto.UserOnlinePeriod, true, now)
	// 2 shares both ways. 5 added me but hid their own last seen.
	// 6 is someone I hid from, and they still share theirs.
	visible := func(id int64) (bool, bool, bool) {
		switch id {
		case 2:
			return true, true, true
		case 5:
			return true, false, true
		case 6:
			return false, true, true
		case 7:
			return false, false, false
		default:
			return false, false, true
		}
	}
	blocked := map[int64]struct{}{9: {}}
	got := statusDeliveries(self, exact, seen, mtproto.UserOnlinePeriod, now, []int64{2, 9}, []int64{5, 6, 7, 2}, visible, blocked)
	byID := map[int64]*mtproto.UserStatus{}
	for _, row := range got {
		byID[row.UserID] = row.Status
	}
	if _, ok := byID[9]; ok {
		t.Fatal("blocked user was included")
	}
	if _, ok := byID[7]; ok {
		t.Fatal("viewer with unknown privacy was included")
	}
	if byID[2].GetPredicateName() != mtproto.Predicate_userStatusOnline || byID[2].GetExpires() != exact.GetExpires() {
		t.Fatalf("mutual exact: %+v", byID[2])
	}
	if byID[5].GetPredicateName() != mtproto.Predicate_userStatusRecently || !byID[5].GetByMe() {
		t.Fatalf("viewer hid own last seen: %+v", byID[5])
	}
	if byID[5].GetPredicateName() == mtproto.Predicate_userStatusOnline || byID[5].GetWasOnline() != 0 {
		t.Fatal("hidden viewer got an exact status")
	}
	if byID[6].GetPredicateName() != mtproto.Predicate_userStatusRecently || byID[6].GetByMe() {
		t.Fatalf("I hid from them: %+v", byID[6])
	}
}

func TestOnlineHeartbeatSlidesExpires(t *testing.T) {
	const seen int64 = 1_700_000_000
	period := int64(mtproto.UserOnlinePeriod)
	prevDeadline := seen + period
	// Android sends account.updateStatus about every 55s while foreground.
	now := seen + 55
	if !mtproto.PresenceOnline(now, seen, mtproto.UserOnlinePeriod) {
		t.Fatal("the heartbeat is still inside the open window")
	}
	newDeadline := now + period
	if !shouldPushStatus(true, true, prevDeadline, newDeadline) {
		t.Fatal("a later online heartbeat must push the new expires")
	}
	if newDeadline <= now || prevDeadline <= now {
		t.Fatal("the contact still holding the previous expires must not already be offline")
	}
	if shouldPushStatus(true, true, newDeadline, newDeadline) {
		t.Fatal("the same expires is not a renewal")
	}
	if !shouldPushStatus(true, false, prevDeadline, now) {
		t.Fatal("going offline is a transition")
	}
	if !shouldPushStatus(false, true, 0, newDeadline) {
		t.Fatal("coming online is a transition")
	}
	if shouldPushStatus(false, false, 0, now) {
		t.Fatal("an offline update while already offline is not a transition")
	}

	// Twelve foreground heartbeats. Each one is pushed, and the contact's
	// expires stays ahead of the clock.
	contactExpires := prevDeadline
	cursor := seen
	for i := 0; i < 12; i++ {
		cursor += 55
		next := cursor + period
		if cursor >= contactExpires {
			t.Fatalf("contact went offline at step %d now %d expires %d", i, cursor, contactExpires)
		}
		if !shouldPushStatus(true, true, contactExpires, next) {
			t.Fatalf("heartbeat %d did not renew", i)
		}
		contactExpires = next
	}
}

func TestAttachForwardContactsFeedsPrivacy(t *testing.T) {
	me := mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User: mtproto.MakeTLUserData(&mtproto.UserData{Id: 1}).To_UserData(),
		KeysPrivacyRules: []*mtproto.PrivacyKeyRules{
			mtproto.MakeTLPrivacyKeyRules(&mtproto.PrivacyKeyRules{
				Key:   mtproto.STATUS_TIMESTAMP,
				Rules: []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueAllowContacts(nil).To_PrivacyRule()},
			}).To_PrivacyKeyRules(),
		},
	}).To_ImmutableUser()
	attachForwardContacts(me, []int64{1, 2, 0})
	if !me.CheckPrivacy(mtproto.STATUS_TIMESTAMP, 2) {
		t.Fatal("a person I added is my contact for status timestamp")
	}
	if me.CheckPrivacy(mtproto.STATUS_TIMESTAMP, 5) {
		t.Fatal("someone who only added me is not my contact")
	}
}
