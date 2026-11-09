package dao

import (
	"os"
	"strings"
	"testing"
)

// Dialog upserts must not assign folder_id. INSERT ... ON DUPLICATE KEY UPDATE
// that writes folder_id (or relies on a zero Go value) puts an archived chat
// back on the main list the next time a message is stored.
func TestDialogUpsertDoesNotResetFolderID(t *testing.T) {
	raw, err := os.ReadFile("../dal/tables/dialogs.xml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, name := range []string{"InsertOrUpdate", "InsertOrUpdateDialog", "UpdateOutboxDialog", "UpdateInboxDialog", "UpdateCustomMap"} {
		body := operationSQL(t, text, name)
		lower := strings.ToLower(body)
		if strings.Contains(lower, "folder_id") {
			t.Fatalf("%s writes folder_id:\n%s", name, body)
		}
	}
	if !strings.Contains(operationSQL(t, text, "UpdatePeerDialogListFolderId"), "folder_id = :folder_id") {
		t.Fatal("folder moves must go through UpdatePeerDialogListFolderId")
	}
}

func operationSQL(t *testing.T, text, name string) string {
	t.Helper()
	needle := `operation name="` + name + `"`
	i := strings.Index(text, needle)
	if i < 0 {
		t.Fatalf("missing %s", name)
	}
	rest := text[i:]
	start := strings.Index(rest, "<sql>")
	end := strings.Index(rest, "</sql>")
	if start < 0 || end < 0 || end < start {
		t.Fatalf("%s sql", name)
	}
	return rest[start+len("<sql>") : end]
}
