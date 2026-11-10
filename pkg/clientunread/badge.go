// Package clientunread records how the official Telegram Android client turns
// dialog.unread_count into the badges the owner compared in the screenshot.
//
// The chat row paints dialog.unread_count (messages). The in-app Chats tab
// paints MessagesStorage.getMainUnreadCount, which counts dialogs that have
// any unread messages. Those two numbers differ when one chat holds several
// unread messages. The server's job is the per-dialog message count.
package clientunread

// Dialog is the slice of a dialog the Android chat list actually reads.
type Dialog struct {
	Unread   int32
	Muted    bool
	Archived bool
}

// RowBadge is DialogCell's unread counter: dialog.unread_count.
func RowBadge(d Dialog) int32 {
	if d.Unread < 0 {
		return 0
	}
	return d.Unread
}

// ChatsTabBadge is MessagesStorage.calcUnreadCounters for the main tab.
//
// Official source (DrKLO/Telegram master, MessagesStorage.calcUnreadCounters):
// each dialog with unread_count > 0 increments a per-kind bucket by one
// (contacts[idx]++, groups[idx]++, ...). The main tab sums those buckets.
// Muted dialogs are left out unless badgeNumberMuted is on, which defaults
// to false. Archived dialogs are left out of the main tab. The count is
// chats, not messages.
//
// The launcher icon is a separate counter (NotificationsController). Its
// "Count unread messages" switch (badgeNumberMessages) defaults to true, so
// the home-screen icon sums messages while this in-app tab still counts chats.
func ChatsTabBadge(dialogs []Dialog, countMuted bool) int {
	n := 0
	for _, d := range dialogs {
		if d.Archived || d.Unread <= 0 {
			continue
		}
		if d.Muted && !countMuted {
			continue
		}
		n++
	}
	return n
}
