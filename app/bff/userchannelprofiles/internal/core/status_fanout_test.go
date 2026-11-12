package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/statusbook"
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

func TestHeartbeatIntervalsDoNotLapseOrReload(t *testing.T) {
	period := int64(mtproto.UserOnlinePeriod)
	for _, gap := range []int64{55, 130, 180, 250, 299} {
		const start int64 = 1_700_000_000
		now := start
		var aud statusbook.Record
		var contact int64
		wasOnline := false
		loads := 0
		load := func() ([]statusDelivery, []statusbook.Viewer, error) {
			loads++
			return []statusDelivery{{UserID: 2}}, []statusbook.Viewer{{ID: 2, Exact: true}}, nil
		}
		var pushedExpires int32
		for step := 0; step < 6; step++ {
			if step > 0 {
				now += gap
			}
			if contact != 0 && now >= contact {
				t.Fatalf("gap %d: contact expired at step %d now %d expires %d", gap, step, now, contact)
			}
			exact := mtproto.MakeTLUserStatusOnline(&mtproto.UserStatus{Expires: int32(now + period)}).To_UserStatus()
			res, err := runStatusHeartbeat(wasOnline, true, now, int32(period), now, now+period, exact, aud, load, func(row statusDelivery) error {
				if row.Status != nil {
					pushedExpires = row.Status.GetExpires()
				}
				return nil
			})
			if err != nil || !res.Save {
				t.Fatalf("gap %d step %d: %+v %v", gap, step, res, err)
			}
			if res.Loaded {
				// counted inside load
			}
			aud = res.Next
			contact = aud.Pushed
			wasOnline = true
		}
		if loads >= 6 {
			t.Fatalf("gap %d reloaded the audience on every heartbeat (%d)", gap, loads)
		}
		if loads != 1 {
			t.Fatalf("gap %d loaded %d times, want the first heartbeat only", gap, loads)
		}
		if contact <= now {
			t.Fatalf("gap %d left contact expires %d at now %d", gap, contact, now)
		}
		if int64(pushedExpires) != contact {
			t.Fatalf("gap %d pushed expires %d, contact has %d", gap, pushedExpires, contact)
		}
	}
}

func TestFailedPushIsRetriedNextHeartbeat(t *testing.T) {
	period := int64(mtproto.UserOnlinePeriod)
	now := int64(1_700_000_000)
	aud := statusbook.Record{}
	fail := true
	pushes := 0
	load := func() ([]statusDelivery, []statusbook.Viewer, error) {
		return []statusDelivery{{UserID: 2}}, []statusbook.Viewer{{ID: 2, Exact: true}}, nil
	}
	for i := 0; i < 2; i++ {
		if i > 0 {
			now += 55
		}
		exact := mtproto.MakeTLUserStatusOnline(&mtproto.UserStatus{Expires: int32(now + period)}).To_UserStatus()
		res, err := runStatusHeartbeat(i > 0, true, now, int32(period), now, now+period, exact, aud, load, func(statusDelivery) error {
			pushes++
			if fail {
				fail = false
				return errors.New("sync")
			}
			return nil
		})
		if err != nil {
			continue
		}
		if res.Save {
			aud = res.Next
		}
	}
	if pushes != 2 {
		t.Fatalf("a failed push was not retried, pushes=%d", pushes)
	}
	if aud.Pushed != now+period {
		t.Fatalf("successful retry did not stick, pushed=%d", aud.Pushed)
	}
}

func TestBookReadErrorDoesNotSkipRenewal(t *testing.T) {
	period := int64(mtproto.UserOnlinePeriod)
	now := int64(1_700_000_000)
	stored := statusbook.Record{Pushed: now + period, HaveList: true, Viewers: []statusbook.Viewer{{ID: 2, Exact: true}}}
	aud := statusbook.OpenRecord(stored, errors.New("redis"))
	loads := 0
	pushes := 0
	exact := mtproto.MakeTLUserStatusOnline(&mtproto.UserStatus{Expires: int32(now + 55 + period)}).To_UserStatus()
	res, err := runStatusHeartbeat(true, true, now+55, int32(period), now+55, now+55+period, exact, aud, func() ([]statusDelivery, []statusbook.Viewer, error) {
		loads++
		return []statusDelivery{{UserID: 2}}, []statusbook.Viewer{{ID: 2, Exact: true}}, nil
	}, func(statusDelivery) error {
		pushes++
		return nil
	})
	if err != nil || !res.Save || loads != 1 || pushes != 1 {
		t.Fatalf("read error skipped renewal: %+v loads %d pushes %d err %v", res, loads, pushes, err)
	}
}

func TestBookWriteErrorStillPushesAndRetries(t *testing.T) {
	period := int64(mtproto.UserOnlinePeriod)
	now := int64(1_700_000_000)
	aud := statusbook.Record{}
	pushes := 0
	load := func() ([]statusDelivery, []statusbook.Viewer, error) {
		return []statusDelivery{{UserID: 2}}, []statusbook.Viewer{{ID: 2, Exact: true}}, nil
	}
	push := func(statusDelivery) error {
		pushes++
		return nil
	}
	exact := mtproto.MakeTLUserStatusOnline(&mtproto.UserStatus{Expires: int32(now + period)}).To_UserStatus()
	res, err := runStatusHeartbeat(false, true, now, int32(period), now, now+period, exact, aud, load, push)
	if err != nil || !res.Save || pushes != 1 {
		t.Fatalf("first push %+v %v pushes %d", res, err, pushes)
	}
	// The book write failed, so the caller keeps the previous record.
	now += 55
	exact = mtproto.MakeTLUserStatusOnline(&mtproto.UserStatus{Expires: int32(now + period)}).To_UserStatus()
	res, err = runStatusHeartbeat(true, true, now, int32(period), now, now+period, exact, aud, load, push)
	if err != nil || !res.Save || pushes != 2 {
		t.Fatalf("write failure skipped the next renewal: %+v %v pushes %d", res, err, pushes)
	}
}

func TestStatusPrivacyChangeStopsExactOnline(t *testing.T) {
	period := int64(mtproto.UserOnlinePeriod)
	now := int64(1_700_000_000)
	const (
		alice = int64(1)
		bob   = int64(2)
	)
	// Bob's cached audience still says Alice may see an exact timestamp.
	stale := statusbook.Record{
		Pushed:   now + period,
		HaveList: true,
		Viewers:  []statusbook.Viewer{{ID: alice, Exact: true}},
	}
	exact := mtproto.MakeTLUserStatusOnline(&mtproto.UserStatus{Expires: int32(now + 55 + period)}).To_UserStatus()
	var staleName string
	if _, err := runStatusHeartbeat(true, true, now+55, int32(period), now+55, now+55+period, exact, stale, func() ([]statusDelivery, []statusbook.Viewer, error) {
		t.Fatal("a warm list must not reload")
		return nil, nil, nil
	}, func(row statusDelivery) error {
		if row.UserID == alice {
			staleName = row.Status.GetPredicateName()
		}
		return nil
	}); err != nil || staleName != mtproto.Predicate_userStatusOnline {
		t.Fatalf("cached exact push = %s %v", staleName, err)
	}
	// Alice changed last seen to nobody, contacts only, or a chat exception.
	// Bob is a contact either way, so his list is forgotten.
	for _, mode := range []string{"nobody", "contacts", "chat exception"} {
		fresh := statusbook.Record{Gen: stale.Gen + 1}
		var got string
		res, err := runStatusHeartbeat(true, true, now+110, int32(period), now+110, now+110+period, exact, fresh, func() ([]statusDelivery, []statusbook.Viewer, error) {
			coarse := mtproto.MakeTLUserStatusRecently(&mtproto.UserStatus{}).To_UserStatus()
			return []statusDelivery{{UserID: alice, Status: coarse}}, []statusbook.Viewer{{ID: alice, ByMe: true}}, nil
		}, func(row statusDelivery) error {
			if row.UserID == alice {
				got = row.Status.GetPredicateName()
			}
			return nil
		})
		if err != nil || !res.Loaded {
			t.Fatalf("%s: forgotten list did not reload: %+v %v", mode, res, err)
		}
		if got == mtproto.Predicate_userStatusOnline || got == "" {
			t.Fatalf("%s: next heartbeat still pushed exact online (%q)", mode, got)
		}
	}
}

func TestClearedListAfterForgetRetryDoesNotPushExact(t *testing.T) {
	period := int64(mtproto.UserOnlinePeriod)
	now := int64(1_700_000_000)
	// The record left after a failed EVAL is still the exact list. The
	// record left after the retry is empty, so the next heartbeat reloads.
	cleared := statusbook.Record{Gen: 2}
	exact := mtproto.MakeTLUserStatusOnline(&mtproto.UserStatus{Expires: int32(now + period)}).To_UserStatus()
	var got string
	res, err := runStatusHeartbeat(true, true, now, int32(period), now, now+period, exact, cleared, func() ([]statusDelivery, []statusbook.Viewer, error) {
		return []statusDelivery{{
			UserID: 1,
			Status: mtproto.MakeTLUserStatusRecently(&mtproto.UserStatus{}).To_UserStatus(),
		}}, []statusbook.Viewer{{ID: 1, ByMe: true}}, nil
	}, func(row statusDelivery) error {
		got = row.Status.GetPredicateName()
		return nil
	})
	if err != nil || !res.Loaded || got == mtproto.Predicate_userStatusOnline || got == "" {
		t.Fatalf("retry still pushed exact: %+v %q %v", res, got, err)
	}
}

func TestStatusEdgesAlwaysReload(t *testing.T) {
	push, reload := planStatusFanout(true, false, true, 300, 10)
	if !push || !reload {
		t.Fatal("going offline reloads")
	}
	push, reload = planStatusFanout(false, true, true, 300, 400)
	if !push || !reload {
		t.Fatal("coming online reloads")
	}
	push, reload = planStatusFanout(false, false, true, 0, 10)
	if push || reload {
		t.Fatal("staying offline does not push")
	}
	push, reload = planStatusFanout(true, true, true, 400, 400)
	if push || reload {
		t.Fatal("the same expires is not pushed again")
	}
}

func TestForgottenAudienceDropsRemovedViewer(t *testing.T) {
	period := int64(mtproto.UserOnlinePeriod)
	now := int64(1_700_000_000)
	stale := statusbook.Record{
		Pushed:   now + period,
		HaveList: true,
		Viewers:  []statusbook.Viewer{{ID: 2, Exact: true}, {ID: 3, Exact: true}},
	}
	var staleIDs []int64
	exact := mtproto.MakeTLUserStatusOnline(&mtproto.UserStatus{Expires: int32(now + 55 + period)}).To_UserStatus()
	_, err := runStatusHeartbeat(true, true, now+55, int32(period), now+55, now+55+period, exact, stale, func() ([]statusDelivery, []statusbook.Viewer, error) {
		t.Fatal("a warm list must not reload")
		return nil, nil, nil
	}, func(row statusDelivery) error {
		staleIDs = append(staleIDs, row.UserID)
		return nil
	})
	if err != nil || len(staleIDs) != 2 {
		t.Fatalf("cached push = %v %v", staleIDs, err)
	}
	// Privacy or block Forget clears the list. The next heartbeat loads
	// again and must not keep the removed viewer.
	fresh := statusbook.Record{}
	var ids []int64
	now += 55
	exact = mtproto.MakeTLUserStatusOnline(&mtproto.UserStatus{Expires: int32(now + period)}).To_UserStatus()
	res, err := runStatusHeartbeat(true, true, now, int32(period), now, now+period, exact, fresh, func() ([]statusDelivery, []statusbook.Viewer, error) {
		return []statusDelivery{{UserID: 2}}, []statusbook.Viewer{{ID: 2, Exact: true}}, nil
	}, func(row statusDelivery) error {
		ids = append(ids, row.UserID)
		return nil
	})
	if err != nil || !res.Loaded {
		t.Fatalf("forgotten list did not reload: %+v %v", res, err)
	}
	if len(ids) != 1 || ids[0] != 2 {
		t.Fatalf("removed viewer was pushed: %v", ids)
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
