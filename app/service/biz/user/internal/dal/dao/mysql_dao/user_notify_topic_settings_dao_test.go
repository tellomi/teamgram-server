package mysql_dao

import (
	"strings"
	"testing"
)

func TestTopicNotifySQLIsASeparateTable(t *testing.T) {
	if !strings.Contains(topicNotifySelectColumns, "top_msg_id") {
		t.Fatal(topicNotifySelectColumns)
	}
	q := strings.ToLower(topicNotifyCacheTargetSQL)
	if !strings.Contains(q, "from user_notify_topic_settings") || strings.Contains(q, "deleted") {
		t.Fatalf("cache target query %s", topicNotifyCacheTargetSQL)
	}
	if strings.Contains(q, "user_notify_settings ") && !strings.Contains(q, "user_notify_topic_settings") {
		t.Fatal("topic rows must not be read from the dialog table")
	}
}
