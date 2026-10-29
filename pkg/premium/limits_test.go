package premium

import "testing"

func TestAboutWithinLimit(t *testing.T) {
	short := make([]rune, AboutLength)
	for i := range short {
		short[i] = '你'
	}
	if !AboutWithinLimit(string(short)) {
		t.Fatalf("Premium biography of %d runes was rejected", AboutLength)
	}
	if AboutWithinLimit(string(short) + "x") {
		t.Fatal("biography past the Premium length was accepted")
	}
	if !AboutWithinLimit("") {
		t.Fatal("empty biography was rejected")
	}
}

func TestStoreColorRoundTrip(t *testing.T) {
	for _, id := range []int32{0, 1, 7, 15} {
		stored := StoreColor(id)
		got, set := LoadColor(stored)
		if !set || got != id {
			t.Fatalf("color %d stored as %d loaded as %d set=%v", id, stored, got, set)
		}
	}
	if StoreColor(-1) != 0 {
		t.Fatal("clearing a color did not store the unset sentinel")
	}
	if _, set := LoadColor(0); set {
		t.Fatal("unset color loaded as set")
	}
}
