package book

import "testing"

func TestBoostURL(t *testing.T) {
	if got := boostURL(peerRef{}); got != "" {
		t.Fatal(got)
	}
	if got := boostURL(peerRef{kind: "channel", id: 42}); got != "https://t.me/boost?c=42" {
		t.Fatal(got)
	}
}
