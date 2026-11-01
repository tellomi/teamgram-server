package notifyack

import (
	"context"
	"testing"
)

func TestTakeClearsAckedPushes(t *testing.T) {
	b := New(nil)
	ctx := context.Background()
	if err := b.Note(ctx, 4, []int32{3, 1, 3, 8}); err != nil {
		t.Fatal(err)
	}
	got, err := b.Take(ctx, 4, 3)
	if err != nil || len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("%v %v", got, err)
	}
	left, err := b.Take(ctx, 4, 100)
	if err != nil || len(left) != 1 || left[0] != 8 {
		t.Fatalf("%v %v", left, err)
	}
	if again, _ := b.Take(ctx, 4, 100); len(again) != 0 {
		t.Fatal(again)
	}
}
