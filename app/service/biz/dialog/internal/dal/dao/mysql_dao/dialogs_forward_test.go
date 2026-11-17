package mysql_dao

import (
	"strings"
	"testing"
)

func TestForwardSetKeepsCursorsMonotonic(t *testing.T) {
	if got := forwardSet("top_message"); got != "top_message = GREATEST(top_message, ?)" {
		t.Fatal(got)
	}
	if got := forwardSet("date2"); got != "date2 = GREATEST(date2, ?)" {
		t.Fatal(got)
	}
	if got := forwardSet("read_inbox_max_id"); got != "read_inbox_max_id = GREATEST(read_inbox_max_id, ?)" {
		t.Fatal(got)
	}
	if got := forwardSet("unread_count"); got != "unread_count = ?" {
		t.Fatal(got)
	}
	for _, part := range []string{
		"GREATEST(top_message, VALUES(top_message))",
		"unread_count = unread_count + VALUES(unread_count)",
		"GREATEST(date2, VALUES(date2))",
	} {
		if !strings.Contains(insertOrUpdateForwardSQL, part) {
			t.Fatal(insertOrUpdateForwardSQL)
		}
	}
}
