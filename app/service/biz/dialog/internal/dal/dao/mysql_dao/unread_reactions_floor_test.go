package mysql_dao

import (
	"strings"
	"testing"
)

func TestUnreadReactionsDoNotGoBelowZero(t *testing.T) {
	if !strings.Contains(updateUnreadCount, "GREATEST(0, unread_reactions_count + (?))") {
		t.Fatal(updateUnreadCount)
	}
}
