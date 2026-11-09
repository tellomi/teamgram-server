package clientunread

import "testing"

// The owner's screenshot: chat "Erel" shows 4, the Chats tab shows 1.
// That is the official Android split, not a server unread_count of 1.
func TestScreenshotMatchesOfficialAndroid(t *testing.T) {
	erel := Dialog{Unread: 4}
	if RowBadge(erel) != 4 {
		t.Fatalf("row = %d", RowBadge(erel))
	}
	if got := ChatsTabBadge([]Dialog{erel}, false); got != 1 {
		t.Fatalf("chats tab = %d, want 1 chat", got)
	}
}

func TestChatsTabSkipsMutedAndArchived(t *testing.T) {
	dialogs := []Dialog{
		{Unread: 4},
		{Unread: 9, Muted: true},
		{Unread: 2, Archived: true},
		{Unread: 0},
	}
	if got := ChatsTabBadge(dialogs, false); got != 1 {
		t.Fatalf("default (muted excluded) = %d", got)
	}
	if got := ChatsTabBadge(dialogs, true); got != 2 {
		t.Fatalf("count muted = %d", got)
	}
}
