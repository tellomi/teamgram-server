package core

import "testing"

func TestBlockedWindowOffsetAndLimit(t *testing.T) {
	off, lim := blockedWindow(-4, 0, blockedPageMax)
	if off != 0 || lim != 100 {
		t.Fatalf("default page %d %d", off, lim)
	}
	off, lim = blockedWindow(20, 10, blockedPageMax)
	if off != 20 || lim != 10 {
		t.Fatalf("explicit page %d %d", off, lim)
	}
	off, lim = blockedWindow(0, 500, blockedPageMax)
	if off != 0 || lim != 100 {
		t.Fatalf("client page cap %d %d", off, lim)
	}
	off, lim = blockedWindow(0, 10000, blockedListMax)
	if off != 0 || lim != 10000 {
		t.Fatalf("full list %d %d", off, lim)
	}
}
