package cleanup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The backfill is opt-in. Startup and deploy code must not execute it.
func TestPresenceBackfillIsNotAutoRun(t *testing.T) {
	body, err := os.ReadFile("optional_user_presences_backfill.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "LEFT JOIN user_presences") || !strings.Contains(text, "WHERE p.user_id IS NULL") {
		t.Fatal("the insert must skip accounts that already have a row")
	}
	if !strings.Contains(text, "p.last_seen_at = 0") || !strings.Contains(text, "p.expires = 0") {
		t.Fatal("the update must only replace a zero placeholder")
	}
	if strings.Contains(text, "FLUSHALL") {
		t.Fatal("the script must not flush redis")
	}

	root := filepath.Clean("../../..")
	var hits []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			if info != nil && (info.Name() == ".git" || info.Name() == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.Contains(path, "optional_user_presences_backfill") {
			return nil
		}
		switch filepath.Ext(path) {
		case ".go", ".yml", ".yaml", ".sh":
		default:
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.Contains(string(b), "optional_user_presences_backfill") {
			hits = append(hits, path)
		}
		return nil
	})
	if len(hits) != 0 {
		t.Fatalf("script is referenced, so it could run on its own: %v", hits)
	}
}
