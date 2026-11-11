package cleanup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInboxPrivatePeerMigrationIsIdempotentUpdate(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("migrate-20261111-inbox-private-peer.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	upper := strings.ToUpper(sql)
	if strings.Contains(upper, "DELETE ") || strings.Contains(upper, "DROP TABLE") {
		t.Fatal("migration must not delete or drop user tables")
	}
	for _, need := range []string{
		"SET peer_id = sender_user_id",
		"peer_type = 2",
		"peer_id = user_id",
		"sender_user_id <> 0",
		"sender_user_id <> user_id",
		"TABLE_NAME = 'messages' OR TABLE_NAME REGEXP '^messages_[0-9]+$'",
		"messages_bak",
	} {
		if !strings.Contains(sql, need) {
			t.Fatalf("migration missing %q", need)
		}
	}
}
