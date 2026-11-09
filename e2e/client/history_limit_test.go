package e2e

import (
	"fmt"
	"testing"
	"time"
)

// TestGetHistoryAllows100 is the official messages.getHistory limit.
// Clients ask for 100. A page of more than 50 must come back.
func TestGetHistoryAllows100(t *testing.T) {
	cfg := requireE2E(t)
	c, _ := loginNamed(t, cfg, ProfileIOS, "folders", "Folders")
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()
	peer := inputPeerSelf()
	stamp := time.Now().UnixNano()
	const n = 55
	for i := 0; i < n; i++ {
		text := fmt.Sprintf("hist100-%d-%d", stamp, i)
		if _, err := c.SendText(ctx, peer, text, nil); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	msgs, err := c.History(ctx, peer, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) <= 50 || len(msgs) > 100 {
		t.Fatalf("getHistory limit 100 returned %d messages", len(msgs))
	}
	over, err := c.History(ctx, peer, 101)
	if err != nil {
		t.Fatal(err)
	}
	if len(over) > 100 {
		t.Fatalf("getHistory limit 101 returned %d messages", len(over))
	}
}
