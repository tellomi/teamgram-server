package chatbox

import (
	"context"
	"testing"
)

func TestPrivateChatContentProtectionIsShared(t *testing.T) {
	b := newBook(nil)
	ctx := context.Background()
	if err := b.SetContentProtected(ctx, 3, 9, true); err != nil {
		t.Fatal(err)
	}
	if !b.ContentProtected(ctx, 9, 3) || !b.ContentProtected(ctx, 3, 9) {
		t.Fatal("pair not protected")
	}
	if b.ContentProtected(ctx, 3, 3) || b.ContentProtected(ctx, 3, 4) {
		t.Fatal("wrong pair protected")
	}
	if err := b.SetContentProtected(ctx, 9, 3, false); err != nil {
		t.Fatal(err)
	}
	if b.ContentProtected(ctx, 3, 9) {
		t.Fatal("still protected")
	}
}
