package mysql_dao

import (
	"strings"
	"testing"
)

func TestUpdateOutboxDialogKeepsTheDraft(t *testing.T) {
	if strings.Contains(updateOutboxDialog, "draft") {
		t.Fatalf("sending a message clears the draft: %s", updateOutboxDialog)
	}
}
