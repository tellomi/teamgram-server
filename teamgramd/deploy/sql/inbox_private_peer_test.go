package cleanup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInboxPrivatePeerMigrationIsIdempotentUpdate(t *testing.T) {
	mig, err := os.ReadFile(filepath.Join("migrate-20261111-inbox-private-peer.sql"))
	if err != nil {
		t.Fatal(err)
	}
	helpers, err := os.ReadFile(filepath.Join("0_migrate_helpers.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(mig)
	body := string(helpers)
	if !strings.Contains(sql, "CALL tellomi_fix_inbox_private_peer();") {
		t.Fatal("migration must CALL tellomi_fix_inbox_private_peer")
	}
	upper := strings.ToUpper(sql)
	if strings.Contains(upper, "DELETE ") || strings.Contains(upper, "DROP TABLE") || strings.Contains(upper, "DROP PROCEDURE") {
		t.Fatal("migrate file must not delete, drop tables, or declare procedures")
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
		if !strings.Contains(body, need) {
			t.Fatalf("helper procedure missing %q", need)
		}
	}
}
