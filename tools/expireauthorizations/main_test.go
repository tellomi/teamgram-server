package main

import (
	"strings"
	"testing"
)

func TestStatementsMatchTheTTLRuleAndCanRollBack(t *testing.T) {
	for name, query := range map[string]string{
		"count":    countSQL,
		"backup":   backupSQL,
		"expire":   expireSQL,
		"tokens":   deleteTokensSQL,
		"rollback": rollbackSQL,
		"table":    backupTable,
	} {
		upper := strings.ToUpper(query)
		if strings.Contains(upper, "FLUSHALL") || strings.Contains(upper, "DROP DATABASE") {
			t.Fatalf("%s is not a bounded update", name)
		}
	}
	if !strings.Contains(staleWhere, "au.date_active > 0") || !strings.Contains(staleWhere, "86400") {
		t.Fatal("stale predicate")
	}
	if !strings.Contains(backupSQL, "auth_users_ttl_backup") || !strings.Contains(backupSQL, "date_created") {
		t.Fatal("backup does not keep the columns rollback restores")
	}
	if !strings.Contains(rollbackSQL, "b.deleted_before") || !strings.Contains(rollbackSQL, "b.date_active") {
		t.Fatal("rollback")
	}
	if !strings.Contains(expireSQL, "au.deleted = 1") || !strings.Contains(expireSQL, "auth_users_ttl_backup") {
		t.Fatal("expire")
	}
}
