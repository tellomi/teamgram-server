package chattitle

import "testing"

func TestClipCountsRunes(t *testing.T) {
	zh := "群" // 3 bytes, one character
	long := "zt" + repeat("🙂", 80)
	if n := len(long); n < 255 {
		t.Fatalf("emoji sample is %d bytes, want a value past the old byte cap", n)
	}
	got := Clip(long)
	if got != long {
		t.Fatalf("82-rune emoji title was clipped to %q", got)
	}
	clipped := Clip(repeat(zh, 200))
	if len([]rune(clipped)) != MaxRunes {
		t.Fatalf("chinese title runes = %d", len([]rune(clipped)))
	}
	if Clip("   ") != "" {
		t.Fatal("blank title should clip to empty")
	}
	about := ClipAbout(repeat(zh, 300))
	if len([]rune(about)) != MaxAboutRunes {
		t.Fatalf("about runes = %d", len([]rune(about)))
	}
}

func repeat(s string, n int) string {
	out := make([]rune, 0, n*len([]rune(s)))
	one := []rune(s)
	for i := 0; i < n; i++ {
		out = append(out, one...)
	}
	return string(out)
}
