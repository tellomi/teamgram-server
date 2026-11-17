package main

import (
	"os"
	"strings"
	"testing"
)

func TestStatementsMatchTheTTLRuleAndCanRollBack(t *testing.T) {
	for name, query := range map[string]string{
		"count":    countSQL,
		"backup":   backupSQL,
		"devices":  deviceBackupSQL,
		"expire":   expireSQL,
		"tokens":   deleteTokensSQL,
		"rollback": rollbackSQL,
		"restore":  restoreTokensSQL,
		"table":    backupTable,
		"devtable": deviceBackupTable,
	} {
		upper := strings.ToUpper(query)
		if strings.Contains(upper, "FLUSHALL") || strings.Contains(upper, "DROP DATABASE") {
			t.Fatalf("%s is not a bounded update", name)
		}
	}
	if !strings.Contains(staleWhere, "au.activity_trusted = 1") || !strings.Contains(staleWhere, "au.date_active > 0") || !strings.Contains(staleWhere, "86400") {
		t.Fatal("stale predicate")
	}
	if !strings.Contains(backupSQL, "auth_users_ttl_backup") || !strings.Contains(backupSQL, "date_created") || !strings.Contains(backupSQL, "activity_trusted") {
		t.Fatal("backup does not keep the columns rollback restores")
	}
	if !strings.Contains(deviceBackupSQL, "token") || !strings.Contains(restoreTokensSQL, "INSERT INTO devices") {
		t.Fatal("device tokens are not backed up")
	}
	if !strings.Contains(rollbackSQL, "b.deleted_before") || !strings.Contains(rollbackSQL, "b.date_active") || !strings.Contains(rollbackSQL, "b.activity_trusted") {
		t.Fatal("rollback")
	}
	if !strings.Contains(expireSQL, "au.deleted = 1") || !strings.Contains(expireSQL, "auth_users_ttl_backup") {
		t.Fatal("expire")
	}
}

func TestDDLIsOutsideTheTransaction(t *testing.T) {
	text, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(text)
	apply := src[strings.Index(src, "func applyRun"):strings.Index(src, "func rollbackRun")]
	rollback := src[strings.Index(src, "func rollbackRun"):]
	for _, name := range []string{"apply", "rollback"} {
		body := apply
		if name == "rollback" {
			body = rollback
		}
		begin := strings.Index(body, "BeginTx")
		ensure := strings.Index(body, "ensureBackupTables")
		if begin < 0 || ensure < 0 || ensure > begin {
			t.Fatalf("%s creates backup tables after the transaction starts", name)
		}
		if strings.Contains(body[begin:], "CREATE TABLE") {
			t.Fatalf("%s contains CREATE TABLE after BeginTx", name)
		}
	}
}
