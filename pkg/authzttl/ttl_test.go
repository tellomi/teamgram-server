package authzttl

import "testing"

func TestDaysUsesDefaultWhenUnset(t *testing.T) {
	if Days(0) != DefaultDays || Days(-1) != DefaultDays {
		t.Fatal("unset ttl")
	}
	if Days(30) != 30 || Days(180) != 180 {
		t.Fatal("stored ttl")
	}
}

func TestExpiredIsInactivityNotLoginAge(t *testing.T) {
	const now = int64(1_700_000_000)
	window := int64(DefaultDays) * Day
	if Expired(now-window+1, now, 0) {
		t.Fatal("one second inside the default window")
	}
	if !Expired(now-window, now, 0) {
		t.Fatal("exactly the default window")
	}
	if Expired(now-10*Day, now, 30) {
		t.Fatal("10 days idle is inside a 30 day window")
	}
	if !Expired(now-30*Day, now, 30) {
		t.Fatal("30 days idle")
	}
	if Expired(0, now, 30) || Expired(-5, now, 30) {
		t.Fatal("missing activity must not expire")
	}
	if Expired(now+10, now, 30) || Expired(now, now, 30) {
		t.Fatal("future or current activity must not expire")
	}
}
