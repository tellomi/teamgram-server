package mysql_dao

import (
	"strings"
	"testing"
)

func TestDateInWindow(t *testing.T) {
	early, mid, late := int32(100), int32(150), int32(200)
	if !DateInWindow(early, 0, mid) || DateInWindow(late, 0, mid) {
		t.Fatal("max_date keeps the earlier message only")
	}
	if DateInWindow(early, mid, 0) || !DateInWindow(late, mid, 0) {
		t.Fatal("min_date keeps the later message only")
	}
	if DateInWindow(early, 0, early-30) || DateInWindow(late, late+3600, 0) {
		t.Fatal("window outside both messages must be empty")
	}
	if !DateInWindow(early, 0, 0) || !DateInWindow(late, early, late) {
		t.Fatal("unbounded and inclusive ends")
	}
}

func TestDialogTextQueryCarriesDates(t *testing.T) {
	q := dialogTextQuery("messages")
	for _, part := range []string{"message like ?", "date2 >= ?", "date2 <= ?", "message != ''"} {
		if !strings.Contains(q, part) {
			t.Fatalf("text query missing %q: %s", part, q)
		}
	}
	from := dialogFromQuery("messages")
	for _, part := range []string{"sender_user_id = ?", "date2 >= ?", "date2 <= ?"} {
		if !strings.Contains(from, part) {
			t.Fatalf("from query missing %q: %s", part, from)
		}
	}
}
