package diffstrict

import "testing"

func TestTooLongStaysOffForANormalGap(t *testing.T) {
	if TooLong(10, 12, 100, 2) {
		t.Fatal("small gap")
	}
	if TooLong(10, 10, 100, 0) {
		t.Fatal("caught up")
	}
	if TooLong(10, 8000, 5000, 3) {
		t.Fatal("gap past the limit still has a short page")
	}
}

func TestTooLongWhenTheLogCannotCatchUp(t *testing.T) {
	if !TooLong(1, 9000, 5000, 0) {
		t.Fatal("empty log and a huge gap")
	}
	if !TooLong(1, 9000, 100, 100) {
		t.Fatal("a full page still leaves a gap past the limit")
	}
}
