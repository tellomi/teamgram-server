package model

import "testing"

func TestAnimatedFileName(t *testing.T) {
	cases := map[string]string{
		"":          "animation.mp4",
		"x.mp4":     "x.mp4",
		"x.gif":     "x.mp4",
		"clip.GIF":  "clip.mp4",
		"x.mp4.mp4": "x.mp4.mp4",
		"../x.mp4":  "x.mp4",
		"animation": "animation.mp4",
	}
	for in, want := range cases {
		if got := AnimatedFileName(in); got != want {
			t.Fatalf("AnimatedFileName(%q) = %q, want %q", in, got, want)
		}
	}
	// The client filename must not grow a second suffix.
	if AnimatedFileName("x.mp4") != "x.mp4" {
		t.Fatal("mp4 name was rewritten")
	}
}
