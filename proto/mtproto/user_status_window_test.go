package mtproto

import "testing"

func TestPresenceWindowMatchesStoredExpires(t *testing.T) {
	const heartbeat int64 = 1_700_000_000
	now := heartbeat + 120

	online := PresenceStatus(heartbeat, UserOnlinePeriod, true, now)
	if online.GetPredicateName() != Predicate_userStatusOnline || int64(online.GetExpires()) != heartbeat+int64(UserOnlinePeriod) {
		t.Fatalf("within 5 minutes: %+v", online)
	}
	// The old reader treated the same row as offline after 60 seconds.
	if now <= heartbeat+60 {
		t.Fatal("fixture must be past the old 60 second window")
	}

	lapsed := PresenceStatus(heartbeat, UserOnlinePeriod, true, heartbeat+int64(UserOnlinePeriod))
	if lapsed.GetPredicateName() != Predicate_userStatusOffline || int64(lapsed.GetWasOnline()) != heartbeat+int64(UserOnlinePeriod) {
		t.Fatalf("after the window: %+v", lapsed)
	}

	explicit := PresenceStatus(heartbeat+10, 0, true, heartbeat+10)
	if explicit.GetPredicateName() != Predicate_userStatusOffline || int64(explicit.GetWasOnline()) != heartbeat+10 {
		t.Fatalf("explicit offline: %+v", explicit)
	}

	missing := PresenceStatus(0, 0, true, now)
	if missing.GetPredicateName() != Predicate_userStatusEmpty {
		t.Fatalf("missing row: %+v", missing)
	}
	if PresenceStatus(0, 0, false, now) != nil {
		t.Fatal("hidden missing row must not invent a timestamp")
	}
}

func TestPresenceCursorRoundTrip(t *testing.T) {
	const heartbeat int64 = 1_700_000_000
	cases := []struct {
		expires int32
		now     int64
	}{
		{expires: UserOnlinePeriod, now: heartbeat + 120},
		{expires: UserOnlinePeriod, now: heartbeat + int64(UserOnlinePeriod) + 5},
		{expires: 0, now: heartbeat + 1},
		{expires: 120, now: heartbeat + 30},
	}
	for _, tc := range cases {
		cursor := PresenceCursor(heartbeat, tc.expires)
		viaUser := PresenceStatus(cursor, UserOnlinePeriod, true, tc.now)
		direct := PresenceStatus(heartbeat, tc.expires, true, tc.now)
		if viaUser.GetPredicateName() != direct.GetPredicateName() || viaUser.GetExpires() != direct.GetExpires() || viaUser.GetWasOnline() != direct.GetWasOnline() {
			t.Fatalf("expires %d now %d: cursor path %+v direct %+v", tc.expires, tc.now, viaUser, direct)
		}
	}
	if PresenceCursor(0, UserOnlinePeriod) != 0 {
		t.Fatal("missing row must stay missing")
	}
}

func TestStatusForViewerReverseRuleAndByMe(t *testing.T) {
	const seen int64 = 1_700_000_000
	now := seen + 30
	exact := PresenceStatus(seen, UserOnlinePeriod, true, now)
	if exact.GetPredicateName() != Predicate_userStatusOnline {
		t.Fatal(exact.GetPredicateName())
	}

	both := StatusForViewer(seen, true, true, now)
	if both.GetPredicateName() != Predicate_userStatusOnline || both.GetByMe() {
		t.Fatalf("both allow: %+v", both)
	}

	viewerHid := StatusForViewer(seen, true, false, now)
	if viewerHid.GetPredicateName() != Predicate_userStatusRecently || !viewerHid.GetByMe() {
		t.Fatalf("viewer hid own last seen: %+v", viewerHid)
	}
	pushed := PushStatusForViewer(exact, seen, UserOnlinePeriod, true, false, now)
	if pushed.GetPredicateName() != viewerHid.GetPredicateName() || pushed.GetByMe() != viewerHid.GetByMe() {
		t.Fatalf("push %+v getUsers %+v", pushed, viewerHid)
	}

	targetHid := PushStatusForViewer(exact, seen, UserOnlinePeriod, false, true, now)
	if targetHid.GetPredicateName() != Predicate_userStatusRecently || targetHid.GetByMe() {
		t.Fatalf("target hid: %+v", targetHid)
	}
	if PushStatusForViewer(exact, seen, UserOnlinePeriod, false, false, now).GetByMe() != true {
		t.Fatal("both hidden still sets by_me because the viewer hid their own")
	}
}

func TestPresenceOnlineTransition(t *testing.T) {
	const seen int64 = 1_700_000_000
	if !PresenceOnline(seen+10, seen, UserOnlinePeriod) {
		t.Fatal("inside the 300s window")
	}
	if PresenceOnline(seen+int64(UserOnlinePeriod), seen, UserOnlinePeriod) {
		t.Fatal("window is exclusive at the end")
	}
	if PresenceOnline(seen+10, seen, 0) || PresenceOnline(seen+10, 0, UserOnlinePeriod) {
		t.Fatal("explicit offline and a missing row are not online")
	}
}

func TestHiddenStatusUsesSameBoundary(t *testing.T) {
	const heartbeat int64 = 1_700_000_000
	hidden := PresenceStatus(heartbeat, UserOnlinePeriod, false, heartbeat+120)
	if hidden.GetPredicateName() != Predicate_userStatusRecently {
		t.Fatalf("online but hidden: %s", hidden.GetPredicateName())
	}
	old := PresenceStatus(heartbeat, UserOnlinePeriod, false, heartbeat+60*60*24*8)
	if old.GetPredicateName() != Predicate_userStatusLastMonth {
		t.Fatalf("week-old hidden: %s", old.GetPredicateName())
	}
}
