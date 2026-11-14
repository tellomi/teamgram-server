package chatbox

import (
	"context"
	"testing"
)

func TestCachedAccessHashMatchesWhatClientsWereGiven(t *testing.T) {
	b := newBook(nil)
	ctx := context.Background()
	if err := b.PutBot(ctx, BotProfile{ID: 50, Name: "x"}); err != nil {
		t.Fatal(err)
	}
	h, ok := b.CachedAccessHash(50)
	if !ok || h != 50 {
		t.Fatalf("zero stored hash is issued as the bot id, got %d ok %v", h, ok)
	}
	if err := b.PutBot(ctx, BotProfile{ID: 51, Name: "y", AccessHash: 77}); err != nil {
		t.Fatal(err)
	}
	h, ok = b.CachedAccessHash(51)
	if !ok || h != 77 {
		t.Fatalf("stored hash %d ok %v", h, ok)
	}
	if _, ok := b.CachedAccessHash(52); ok {
		t.Fatal("unknown bot")
	}
}
