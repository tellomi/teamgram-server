package push

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func TestMySQLLoggedOutDevicesAreNotListedAndCleanupIsIdempotent(t *testing.T) {
	db := openPushTestDB(t)
	ctx := context.Background()
	if _, err := db.Exec(`
CREATE TABLE devices (
  id BIGINT NOT NULL AUTO_INCREMENT,
  auth_key_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  token_type INT NOT NULL,
  token VARCHAR(512) NOT NULL,
  no_muted TINYINT NOT NULL DEFAULT 0,
  locked_period INT NOT NULL DEFAULT 0,
  app_sandbox TINYINT NOT NULL DEFAULT 0,
  secret VARCHAR(1024) NOT NULL DEFAULT '',
  other_uids VARCHAR(1024) NOT NULL DEFAULT '',
  lang_code VARCHAR(32) NOT NULL DEFAULT '',
  state TINYINT NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY auth_key_id (auth_key_id, user_id, token_type)
)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
CREATE TABLE auth_users (
  id BIGINT NOT NULL AUTO_INCREMENT,
  auth_key_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  deleted TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY auth_key_id (auth_key_id, user_id)
)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
CREATE TABLE auths (
  id BIGINT NOT NULL AUTO_INCREMENT,
  auth_key_id BIGINT NOT NULL,
  lang_code VARCHAR(32) NOT NULL DEFAULT '',
  system_lang_code VARCHAR(32) NOT NULL DEFAULT '',
  deleted TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY auth_key_id (auth_key_id)
)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO auth_users (auth_key_id, user_id, deleted) VALUES
(10, 9, 0),
(11, 9, 1),
(12, 9, 0),
(10, 8, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO auths (auth_key_id, lang_code, system_lang_code, deleted) VALUES (10, 'zh-hans', 'zh-CN', 0)`); err != nil {
		t.Fatal(err)
	}

	store := &MySQL{db: db}
	secret := bytesRepeat(8)
	for _, d := range []Device{
		{AuthKeyID: 10, UserID: 9, TokenType: TokenAPNS, Token: "newest", Secret: secret, LangCode: "en"},
		{AuthKeyID: 12, UserID: 9, TokenType: TokenAPNS, Token: "older", Secret: secret},
		{AuthKeyID: 11, UserID: 9, TokenType: TokenAPNS, Token: "logged-out", Secret: secret},
		{AuthKeyID: 99, UserID: 9, TokenType: TokenAPNS, Token: "no-session", Secret: secret},
		{AuthKeyID: 10, UserID: 8, TokenType: TokenFCM, Token: "other-user"},
	} {
		if err := store.Upsert(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC)
	if _, err := db.Exec(`UPDATE devices SET updated_at = ? WHERE token = 'older'`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE devices SET updated_at = ? WHERE token = 'newest'`, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	list, err := store.ListByUser(ctx, 9)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Token != "newest" || list[1].Token != "older" {
		t.Fatalf("list %+v", tokensOf(list))
	}
	if list[0].LangCode != "zh-hans" || list[0].SystemLangCode != "zh-CN" {
		t.Fatalf("lang %q %q", list[0].LangCode, list[0].SystemLangCode)
	}
	if len(list[0].Secret) != len(secret) {
		t.Fatal("secret was not loaded")
	}

	if _, err := db.Exec(`UPDATE auth_users SET deleted = 1 WHERE auth_key_id = 12 AND user_id = 9`); err != nil {
		t.Fatal(err)
	}
	list, err = store.ListByUser(ctx, 9)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Token != "newest" {
		t.Fatalf("after logout %+v", tokensOf(list))
	}

	if err := store.DeleteAuthKey(ctx, 9, 10); err != nil {
		t.Fatal(err)
	}
	list, _ = store.ListByUser(ctx, 9)
	if len(list) != 0 {
		t.Fatalf("delete auth key left %+v", tokensOf(list))
	}
	other, err := store.ListByUser(ctx, 8)
	if err != nil || len(other) != 1 || other[0].Token != "other-user" {
		t.Fatalf("other user %+v %v", tokensOf(other), err)
	}

	matched, deleted, err := CleanupStaleDevices(ctx, db, false)
	if err != nil {
		t.Fatal(err)
	}
	if matched != 3 || deleted != 0 {
		t.Fatalf("dry-run matched %d deleted %d", matched, deleted)
	}
	var still int
	if err := db.QueryRow(`SELECT COUNT(*) FROM devices WHERE token IN ('logged-out','no-session','older')`).Scan(&still); err != nil {
		t.Fatal(err)
	}
	if still != 3 {
		t.Fatalf("dry-run deleted rows, left %d", still)
	}
	matched, deleted, err = CleanupStaleDevices(ctx, db, true)
	if err != nil {
		t.Fatal(err)
	}
	if matched != 3 || deleted != 3 {
		t.Fatalf("apply matched %d deleted %d", matched, deleted)
	}
	matched, deleted, err = CleanupStaleDevices(ctx, db, true)
	if err != nil {
		t.Fatal(err)
	}
	if matched != 0 || deleted != 0 {
		t.Fatalf("second apply matched %d deleted %d", matched, deleted)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM devices WHERE user_id = 8`).Scan(&still); err != nil {
		t.Fatal(err)
	}
	if still != 1 {
		t.Fatalf("cleanup removed a live other-user row, left %d", still)
	}
}

func tokensOf(list []Device) []string {
	out := make([]string, len(list))
	for i := range list {
		out[i] = list[i].Token
	}
	return out
}

func openPushTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEAMGRAM_MYSQL_DSN")
	if dsn == "" {
		dsn = "root:@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true&multiStatements=true"
	}
	admin, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("mysql not available: %v", err)
	}
	t.Cleanup(func() { admin.Close() })
	if err = admin.Ping(); err != nil {
		t.Skipf("mysql not available: %v", err)
	}
	const schema = "teamgram_push_logout_devices"
	if _, err = admin.Exec("DROP DATABASE IF EXISTS " + schema); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec("CREATE DATABASE " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS " + schema) })
	db, err := sql.Open("mysql", dsnWithSchema(dsn, schema))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = db.Ping(); err != nil {
		t.Fatal(err)
	}
	return db
}

func dsnWithSchema(dsn, schema string) string {
	if i := stringsLastSlash(dsn); i >= 0 {
		rest := dsn[i+1:]
		if q := stringsIndex(rest, "?"); q >= 0 {
			return dsn[:i+1] + schema + rest[q:]
		}
		return dsn[:i+1] + schema
	}
	return dsn + "/" + schema
}

func stringsLastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

func stringsIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
