package mysql_dao

import (
	"strings"
	"testing"
)

func TestTopicDraftSQLKeepsDialogDrafts(t *testing.T) {
	for _, q := range []string{topicDraftUpsertSQL, topicDraftSelectUserSQL, topicDraftClearOneSQL, topicDraftClearUserSQL} {
		if strings.Contains(q, "dialogs") {
			t.Fatalf("topic draft SQL touches dialogs: %s", q)
		}
		if strings.Contains(strings.ToLower(q), "delete ") {
			t.Fatalf("topic draft SQL deletes rows: %s", q)
		}
	}
	if !strings.Contains(topicDraftUpsertSQL, "top_msg_id") || !strings.Contains(topicDraftUpsertSQL, "on duplicate key update") {
		t.Fatal(topicDraftUpsertSQL)
	}
	if !strings.Contains(topicDraftClearOneSQL, "top_msg_id = ?") || !strings.Contains(topicDraftClearOneSQL, "draft_type = 2") {
		t.Fatal(topicDraftClearOneSQL)
	}
	if !strings.Contains(topicDraftSelectUserSQL, "draft_type > 0") || !strings.Contains(topicDraftSelectUserSQL, "top_msg_id") {
		t.Fatal(topicDraftSelectUserSQL)
	}
}
