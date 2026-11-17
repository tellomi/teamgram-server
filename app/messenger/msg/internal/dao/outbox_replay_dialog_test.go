package dao

import (
	"strings"
	"testing"
)

func TestOutboxDialogWrite(t *testing.T) {
	// First insert of a live row keeps the historical dialog write.
	if top, unread := outboxDialogWrite(false, false); !top || !unread {
		t.Fatalf("first live insert top=%v unread=%v", top, unread)
	}
	// Delete can commit before the dialog RPC. That id is not a top.
	if top, unread := outboxDialogWrite(false, true); top || unread {
		t.Fatalf("deleted first insert top=%v unread=%v", top, unread)
	}
	// Replay of a live row still advances top. The dialog RPC is outside
	// the insert transaction, so this is the crash recovery. It must not
	// assign unread_count. An older id cannot rewind top: the dialog update
	// uses GREATEST, covered by dialogs_forward_test.go.
	if top, unread := outboxDialogWrite(true, false); !top || unread {
		t.Fatalf("live replay top=%v unread=%v", top, unread)
	}
	// Replay of the deleted newest id must not move top back up.
	if top, unread := outboxDialogWrite(true, true); top || unread {
		t.Fatalf("deleted replay top=%v unread=%v", top, unread)
	}
}

func TestSavedPeerDialogStaysForwardOnly(t *testing.T) {
	if savedPeerDialogSQL == "" {
		t.Fatal("missing saved dialog statement")
	}
	for _, want := range []string{
		"deleted = if(top_message < values(top_message), 0, deleted)",
		"top_message = greatest(top_message, values(top_message))",
	} {
		if !strings.Contains(savedPeerDialogSQL, want) {
			t.Fatalf("statement %q missing %q", savedPeerDialogSQL, want)
		}
	}
	if strings.Contains(savedPeerDialogSQL, "top_message = values(top_message)") {
		t.Fatal("saved dialog top is an absolute assignment")
	}
}
