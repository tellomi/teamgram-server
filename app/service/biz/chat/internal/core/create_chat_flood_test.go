package core

import "testing"

func TestCreateChatFloodWaitIsTimeStillLeft(t *testing.T) {
	if got := createChatFloodWait(100, 100); got != createChatFlood {
		t.Fatalf("same second: %d", got)
	}
	if got := createChatFloodWait(107, 100); got != 3 {
		t.Fatalf("7s elapsed, 3s left: %d", got)
	}
	if got := createChatFloodWait(109, 100); got != 1 {
		t.Fatalf("9s elapsed: %d", got)
	}
	if got := createChatFloodWait(110, 100); got != 0 {
		t.Fatalf("gap passed: %d", got)
	}
	if got := createChatFloodWait(90, 100); got != createChatFlood {
		t.Fatalf("clock behind: %d", got)
	}
}
