package premium

import "testing"

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
