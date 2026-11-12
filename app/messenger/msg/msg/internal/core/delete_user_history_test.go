package core

import (
	"os"
	"strings"
	"testing"
)

func TestHistoryClearDoesNotCallEditUserOutboxMessage(t *testing.T) {
	for _, path := range []string{
		"delete_user_history.go",
		"../../../inbox/internal/core/inbox.deleteUserHistoryToInbox_handler.go",
	} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), ".EditUserOutboxMessage(") {
			t.Fatalf("%s edits history clear through EditUserOutboxMessage, which allocates another pts", path)
		}
		if !strings.Contains(string(body), "SaveEditedMessage") {
			t.Fatalf("%s does not save the history-clear body without a pts allocation", path)
		}
	}
}
