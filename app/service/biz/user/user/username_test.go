package user

import "testing"

func TestCheckUsernameInvalid(t *testing.T) {
	ok := []string{"abc", "AbC", "a_b", "a1b", "n" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	bad := []string{"", "ab", "a", "1abc", "_abc", "abc_", "a b", "a.b", "abcdefghijabcdefghijabcdefghijabc"}
	if len(ok[4]) != MaxUsernameLen {
		t.Fatalf("fixture length %d", len(ok[4]))
	}
	if len(bad[len(bad)-1]) != MaxUsernameLen+1 {
		t.Fatalf("long fixture %d", len(bad[len(bad)-1]))
	}
	for _, name := range ok {
		if !CheckUsernameInvalid(name) {
			t.Errorf("valid username rejected: %q", name)
		}
	}
	for _, name := range bad {
		if CheckUsernameInvalid(name) {
			t.Errorf("invalid username accepted: %q", name)
		}
	}
}
