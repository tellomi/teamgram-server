package core

import "testing"

func TestNonNegativeSeq(t *testing.T) {
	if got := nonNegativeSeq(-1); got != 0 {
		t.Fatalf("missing seq %d", got)
	}
	if got := nonNegativeSeq(0); got != 0 {
		t.Fatalf("zero seq %d", got)
	}
	if got := nonNegativeSeq(7); got != 7 {
		t.Fatalf("stored seq %d", got)
	}
}
