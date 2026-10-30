package mysql_dao

import (
	"strings"
	"testing"
)

func TestInsertEmailUserSQLWritesNullPhone(t *testing.T) {
	if !strings.Contains(insertEmailUserSQL, "NULL") || !strings.Contains(insertEmailUserSQL, "email") {
		t.Fatal(insertEmailUserSQL)
	}
	lower := strings.ToLower(insertEmailUserSQL)
	if strings.Contains(lower, "?, ?, ?, ?, ?, ?, '',") || strings.Contains(lower, "?, ?, ?, ?, ?, ?, \"\",") {
		t.Fatal(insertEmailUserSQL)
	}
}
