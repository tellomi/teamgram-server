package verify

import "testing"

func TestDigitsFromBytesKeepsLeadingZero(t *testing.T) {
	got := DigitsFromBytes([]byte{0, 11, 22, 33, 44, 55})
	if got != "012345" {
		t.Fatalf("digits %q", got)
	}
	if len(got) != 6 {
		t.Fatalf("len %d", len(got))
	}
}

func TestRandomDigitsLength(t *testing.T) {
	for _, n := range []int{4, 5, 6, 8} {
		code := RandomDigits(n)
		if len(code) != n {
			t.Fatalf("RandomDigits(%d) = %q", n, code)
		}
		for _, c := range code {
			if c < '0' || c > '9' {
				t.Fatalf("non-digit %q", code)
			}
		}
	}
	if got := RandomDigits(3); len(got) != 6 {
		t.Fatalf("out of range length %d (%q)", len(got), got)
	}
}

func TestNormalizeCodeLengthDefaultSix(t *testing.T) {
	if NormalizeCodeLength(0) != 6 || NormalizeCodeLength(9) != 6 {
		t.Fatal("default length is 6")
	}
	if NormalizeCodeLength(5) != 5 || NormalizeCodeLength(6) != 6 {
		t.Fatal("explicit 5 and 6 are kept")
	}
}
