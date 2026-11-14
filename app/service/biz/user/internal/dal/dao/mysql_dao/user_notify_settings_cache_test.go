package mysql_dao

import (
	"strings"
	"testing"
)

func TestNotifyCacheTargetSQLIncludesSoftDeletedRows(t *testing.T) {
	q := strings.ToLower(notifySettingsCacheTargetSQL)
	if !strings.Contains(q, "from user_notify_settings") || !strings.Contains(q, "user_id = ?") {
		t.Fatalf("query %s", notifySettingsCacheTargetSQL)
	}
	if strings.Contains(q, "deleted") {
		t.Fatalf("reset must still see soft-deleted rows so it can name their cache keys: %s", notifySettingsCacheTargetSQL)
	}
}
