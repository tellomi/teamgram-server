package user

import "testing"

func TestCheckUsernameInvalid(t *testing.T) {
	ok := []string{"abcd", "AbCd", "a_bc", "a1bc", "n" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	bad := []string{"", "abc", "ab", "a", "1abc", "_abc", "abc_", "a b", "a.b", "abcdefghijabcdefghijabcdefghijabc"}
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

func TestGetBotIdByNameFoldsCase(t *testing.T) {
	if got := GetBotIdByName("BotFather"); got != BotFatherId {
		t.Fatalf("BotFather %d", got)
	}
	if got := GetBotIdByName("@botfather"); got != BotFatherId {
		t.Fatalf("botfather %d", got)
	}
	if got := GetBotIdByName("BING"); got != BotBingId {
		t.Fatalf("BING %d", got)
	}
	if got := GetBotIdByName("GIF"); got != BotGifId {
		t.Fatalf("GIF %d", got)
	}
	if got := GetBotIdByName("nobody"); got != 0 {
		t.Fatalf("nobody %d", got)
	}
}

func TestFoldUsernameAllowsShortBots(t *testing.T) {
	name, ok := FoldUsername("GIF")
	if !ok || name != "gif" {
		t.Fatalf("GIF %q %v", name, ok)
	}
	name, ok = FoldUsername("  @BotFather ")
	if !ok || name != "botfather" {
		t.Fatalf("BotFather %q %v", name, ok)
	}
	if _, ok = FoldUsername("ab"); ok {
		t.Fatal("short name was accepted")
	}
	if _, ok = FoldUsername(""); ok {
		t.Fatal("empty name was accepted")
	}
	name, ok = FoldUsername("Tellomi")
	if !ok || name != "tellomi" {
		t.Fatalf("Tellomi %q %v", name, ok)
	}
}
