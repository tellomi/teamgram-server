package cleanup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInboxPrivatePeerMigrationIsIdempotentUpdate(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("migrate-20261126-inbox-private-peer.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	upper := strings.ToUpper(sql)
	if strings.Contains(upper, "DELETE ") || strings.Contains(upper, "DROP ") {
		t.Fatal("migration must not delete or drop rows")
	}
	for _, need := range []string{
		"UPDATE messages",
		"SET peer_id = sender_user_id",
		"peer_type = 2",
		"peer_id = user_id",
		"sender_user_id <> 0",
		"sender_user_id <> user_id",
	} {
		if !strings.Contains(sql, need) {
			t.Fatalf("migration missing %q", need)
		}
	}
}
