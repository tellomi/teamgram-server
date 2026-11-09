package main

import (
	"context"
	"strings"
	"testing"
)

func TestRunRequiresDSN(t *testing.T) {
	err := run(context.Background(), "", true, false)
	if err == nil || !strings.Contains(err.Error(), "TEAMGRAM_MYSQL_DSN") {
		t.Fatal(err)
	}
}

func TestBackupInstructionsNameTheDump(t *testing.T) {
	text := backupInstructions()
	for _, part := range []string{"mysqldump", "devices", "-backup-done", "docs/push.md", "secrets"} {
		if !strings.Contains(text, part) {
			t.Fatalf("instructions missing %q\n%s", part, text)
		}
	}
}
