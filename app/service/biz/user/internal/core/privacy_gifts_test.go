package core

import "testing"

func TestGiftPrivacyRoundTrip(t *testing.T) {
	in := &struct {
		unlimited, limited, unique, premium, channels bool
	}{true, false, true, false, true}
	_ = in
	raw := encodeGifts(nil)
	if raw != "" || decodeGifts(raw) != nil {
		t.Fatal("nil gifts")
	}
	stored := encodeGifts(decodeGifts(`{"unlimited":true,"unique":true,"channels":true}`))
	back := decodeGifts(stored)
	if back == nil || !back.GetDisallowUnlimitedStargifts() || back.GetDisallowLimitedStargifts() || !back.GetDisallowUniqueStargifts() || !back.GetDisallowStargiftsFromChannels() {
		t.Fatalf("%+v", back)
	}
}
