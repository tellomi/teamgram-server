package mtproto

import "testing"

func TestFootballDiceStaysInOfficialRange(t *testing.T) {
	for i := 0; i < 200; i++ {
		if v := DiceFace("⚽"); v < 1 || v > 5 {
			t.Fatalf("football %d", v)
		}
		if v := DiceFace("🏀"); v < 1 || v > 5 {
			t.Fatalf("basketball %d", v)
		}
		if v := DiceFace("🎲"); v < 1 || v > 6 {
			t.Fatalf("die %d", v)
		}
	}
}
