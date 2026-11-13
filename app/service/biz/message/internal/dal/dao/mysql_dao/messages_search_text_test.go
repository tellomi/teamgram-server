package mysql_dao

import (
	"strings"
	"testing"
)

func TestLikeSubstringEscapesAndGlobalSQLIsCaseInsensitive(t *testing.T) {
	if got := likeSubstring(`Hello`); got != `%Hello%` {
		t.Fatalf("plain %s", got)
	}
	if got := likeSubstring(`100%_a\b`); got != `%100\%\_a\\b%` {
		t.Fatalf("escaped %s", got)
	}
	sql := searchGlobalTextSelect + searchGlobalTextCursor + searchGlobalTextOrder
	if !strings.Contains(sql, "utf8mb4_unicode_ci") {
		t.Fatal("search must ignore case")
	}
	if !strings.Contains(sql, "message = ''") || !strings.Contains(sql, "message_data") {
		t.Fatal("empty plaintext column must fall back to message_data")
	}
	if strings.Contains(searchGlobalTextSelect, "sender_user_id =") {
		t.Fatal("search must include inbox and outbox")
	}
	if !strings.Contains(sql, "date2 < ?") || !strings.Contains(sql, "peer_type > ?") || !strings.Contains(sql, "peer_id > ?") {
		t.Fatal("cursor must be the date, peer, id tuple")
	}
	if !strings.Contains(sql, "peer_type = ? and peer_id = ? and user_message_box_id < ?") {
		t.Fatal("box id is only the tie-break on the cursor peer")
	}
	if strings.Contains(searchGlobalTextSelect, "user_message_box_id <") {
		t.Fatal("the first page must not bound box ids")
	}
	if !strings.Contains(searchGlobalTextOrder, "order by date2 desc, peer_type asc, peer_id asc, user_message_box_id desc") {
		t.Fatal(searchGlobalTextOrder)
	}
}
