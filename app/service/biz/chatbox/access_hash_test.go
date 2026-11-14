package chatbox

import (
	"context"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/accesshash"
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

func TestIssuingLoadsTheRowInsteadOfTheBotID(t *testing.T) {
	defer accesshash.SetMode(accesshash.ModeLog)()
	defer accesshash.SetUserLookup(nil)
	accesshash.SetUserLookup(func(ctx context.Context, ids []int64) (map[int64]int64, error) {
		return map[int64]int64{50: 88}, nil
	})
	b := newBook(nil)
	ctx := context.Background()
	if err := b.PutBot(ctx, BotProfile{ID: 50, Name: "x"}); err != nil {
		t.Fatal(err)
	}
	h, ok := b.CachedAccessHash(50)
	if !ok || h != 88 {
		t.Fatalf("row hash %d ok %v", h, ok)
	}
	if got := (botProfile{ID: 50}).accessHash(); got != 88 {
		t.Fatalf("issued %d", got)
	}
	accesshash.SetUserLookup(func(ctx context.Context, ids []int64) (map[int64]int64, error) {
		return nil, nil
	})
	if _, ok := b.CachedAccessHash(50); ok {
		t.Fatal("a zero row must not fall back to the bot id")
	}
	if got := (botProfile{ID: 50}).accessHash(); got != 0 {
		t.Fatalf("issued zero row as %d", got)
	}
}
