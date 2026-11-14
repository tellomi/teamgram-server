package chatbox

import (
	"context"
	"strconv"
)

func protectKey(a, b int64) string {
	if a > b {
		a, b = b, a
	}
	return "nofwd:" + strconv.FormatInt(a, 10) + ":" + strconv.FormatInt(b, 10)
}

// SetContentProtected stores messages.toggleNoForwards for a private chat.
// Both users share one flag. Forwarding a message from that chat is
// CHAT_FORWARDS_RESTRICTED while it is on.
func (b *Book) SetContentProtected(ctx context.Context, a, bUser int64, enabled bool) error {
	if b == nil || a == 0 || bUser == 0 || a == bUser {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	key := protectKey(a, bUser)
	if b.protect == nil {
		b.protect = map[string]bool{}
	}
	if !enabled {
		delete(b.protect, key)
	} else {
		b.protect[key] = true
	}
	return b.save(ctx, key, enabled)
}

// ContentProtected reports whether this private chat forbids forwards.
func (b *Book) ContentProtected(ctx context.Context, a, bUser int64) bool {
	if b == nil || a == 0 || bUser == 0 || a == bUser {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	key := protectKey(a, bUser)
	var stored bool
	if err := b.load(ctx, key, &stored); err == nil && b.db != nil {
		if b.protect == nil {
			b.protect = map[string]bool{}
		}
		if stored {
			b.protect[key] = true
		}
		return stored
	}
	return b.protect[key]
}
