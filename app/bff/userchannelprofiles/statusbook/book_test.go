package statusbook

import (
	"context"
	"testing"
)

func TestRememberForgetAndMissing(t *testing.T) {
	b := New(nil)
	ctx := context.Background()
	got, err := b.Pushed(ctx, 7)
	if err != nil || got != 0 {
		t.Fatalf("missing mark = %d %v", got, err)
	}
	if err = b.Remember(ctx, 7, 1_700_000_300); err != nil {
		t.Fatal(err)
	}
	got, err = b.Pushed(ctx, 7)
	if err != nil || got != 1_700_000_300 {
		t.Fatalf("remembered = %d %v", got, err)
	}
	b.Forget(ctx, 7)
	got, err = b.Pushed(ctx, 7)
	if err != nil || got != 0 {
		t.Fatalf("forgotten = %d %v", got, err)
	}
}
