package emojiblock

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultIsTaiwanFlag(t *testing.T) {
	t.Cleanup(Reload)
	t.Setenv(EnvExtra, "")
	t.Setenv(EnvFile, "")
	Reload()

	list := List()
	if len(list) != 1 || list[0] != TaiwanFlag {
		t.Fatalf("list = %#v", list)
	}
	runes := []rune(TaiwanFlag)
	if len(runes) != 2 || runes[0] != 0x1F1F9 || runes[1] != 0x1F1FC {
		t.Fatalf("taiwan flag runes = %#x", runes)
	}
	if !Blocked(TaiwanFlag) || !Blocked(TaiwanFlag+"\uFE0F") {
		t.Fatal("taiwan flag was not blocked")
	}
	if Blocked("") || Blocked("\U0001F44D") {
		t.Fatal("unrelated emoji was blocked")
	}
	if Fingerprint() != BuiltinFingerprint() {
		t.Fatal("default fingerprint drifted from the built-in list")
	}
}

func TestExtraAndFileExtendTheDefault(t *testing.T) {
	t.Cleanup(Reload)
	dir := t.TempDir()
	path := filepath.Join(dir, "emoji-blocklist.txt")
	body := "# comment\n\n1F525 # fire\n\u2764\uFE0F\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvExtra, "U+1F1F9 U+1F1FC, 1F44D")
	t.Setenv(EnvFile, path)
	Reload()

	list := List()
	want := []string{TaiwanFlag, "\U0001F44D", "\U0001F525", "\u2764"}
	if len(list) != len(want) {
		t.Fatalf("list = %#v", list)
	}
	for i, emo := range want {
		if list[i] != emo {
			t.Fatalf("list[%d] = %q, want %q (%#v)", i, list[i], emo, list)
		}
	}
	if !Blocked("\u2764") || !Blocked("\u2764\uFE0F") || !Blocked("\U0001F44D") {
		t.Fatal("extended entries did not match")
	}
	if Fingerprint() == BuiltinFingerprint() {
		t.Fatal("extended list kept the built-in fingerprint")
	}
}

func TestMissingFileKeepsTheDefault(t *testing.T) {
	t.Cleanup(Reload)
	t.Setenv(EnvExtra, "")
	t.Setenv(EnvFile, filepath.Join(t.TempDir(), "missing.txt"))
	Reload()
	list := List()
	if len(list) != 1 || list[0] != TaiwanFlag {
		t.Fatalf("list = %#v", list)
	}
}

func TestCodepointSequenceStaysOneEntry(t *testing.T) {
	t.Cleanup(Reload)
	t.Setenv(EnvFile, "")
	t.Setenv(EnvExtra, "1F1E8-1F1F3")
	Reload()
	china := "\U0001F1E8\U0001F1F3"
	list := List()
	if len(list) != 2 || list[0] != TaiwanFlag || list[1] != china {
		t.Fatalf("list = %#v", list)
	}
	if !Blocked(china) {
		t.Fatal("code-point sequence was not blocked")
	}
}
