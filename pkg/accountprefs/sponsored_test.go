package accountprefs

import (
	"context"
	"testing"
)

func TestSponsoredMemoryRoundTrip(t *testing.T) {
	mu.Lock()
	prevDB := db
	db = nil
	mem = map[int64]bool{}
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		db = prevDB
		mem = map[int64]bool{}
		mu.Unlock()
	})
	if Sponsored(context.Background(), 7) {
		t.Fatal("default is off")
	}
	if err := SetSponsored(context.Background(), 7, true); err != nil {
		t.Fatal(err)
	}
	if !Sponsored(context.Background(), 7) {
		t.Fatal("toggle did not stick")
	}
	if err := SetSponsored(context.Background(), 7, false); err != nil {
		t.Fatal(err)
	}
	if Sponsored(context.Background(), 7) {
		t.Fatal("toggle off did not stick")
	}
}
