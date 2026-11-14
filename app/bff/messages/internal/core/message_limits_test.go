package core

import "testing"

func TestTextAndCaptionLimits(t *testing.T) {
	if textTooLong(repeat("a", 4096)) || !textTooLong(repeat("a", 4097)) {
		t.Fatal("message length")
	}
	// One emoji is two UTF-16 units. 2049 of them is past 4096.
	if !textTooLong(repeat("🙂", 2049)) {
		t.Fatal("emoji message length")
	}
	if captionTooLong(repeat("a", 1024), false) || !captionTooLong(repeat("a", 1025), false) {
		t.Fatal("caption")
	}
	if captionTooLong(repeat("a", 2048), true) || !captionTooLong(repeat("a", 2049), true) {
		t.Fatal("premium caption")
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
