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
	if !strings.Contains(searchGlobalTextSQL, "utf8mb4_unicode_ci") {
		t.Fatal("search must ignore case")
	}
	if !strings.Contains(searchGlobalTextSQL, "message = ''") || !strings.Contains(searchGlobalTextSQL, "message_data") {
		t.Fatal("empty plaintext column must fall back to message_data")
	}
	if strings.Contains(searchGlobalTextSQL, "sender_user_id =") {
		t.Fatal("search must include inbox and outbox")
	}
}
