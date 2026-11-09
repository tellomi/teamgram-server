package core

import "testing"

func TestContactHitAllowedHonorsFlags(t *testing.T) {
	all := []contactHit{contactUser, contactBot, contactGroup, contactChannel}
	for _, hit := range all {
		if !contactHitAllowed(false, false, hit) {
			t.Fatalf("default dropped %d", hit)
		}
	}
	if contactHitAllowed(true, false, contactUser) || contactHitAllowed(true, false, contactGroup) || contactHitAllowed(true, false, contactBot) {
		t.Fatal("broadcasts should be channels only")
	}
	if !contactHitAllowed(true, false, contactChannel) {
		t.Fatal("broadcasts missed a channel")
	}
	if !contactHitAllowed(false, true, contactBot) || contactHitAllowed(false, true, contactChannel) || contactHitAllowed(false, true, contactUser) {
		t.Fatal("bots flag")
	}
	if !contactHitAllowed(true, true, contactBot) || !contactHitAllowed(true, true, contactChannel) || contactHitAllowed(true, true, contactGroup) {
		t.Fatal("both flags")
	}
}
