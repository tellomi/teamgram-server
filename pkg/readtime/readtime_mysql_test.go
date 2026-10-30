package readtime

import (
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// TestOutboxReadDateQueryIsNotEpoch is the regression for clients showing
// 08:00. A checkpoint stored with read_outbox_max_date = 0 still has
// updated_at, and the query messages.getOutboxReadDate runs must not return 0.
func TestOutboxReadDateQueryIsNotEpoch(t *testing.T) {
	dsn := os.Getenv("TEAMGRAM_MYSQL_DSN")
	if dsn == "" {
		dsn = "root:@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true&multiStatements=true"
	}
	admin, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("mysql not available: %v", err)
	}
	defer admin.Close()
	if err = admin.Ping(); err != nil {
		t.Skipf("mysql not available: %v", err)
	}

	const schema = "teamgram_outbox_read_date"
	if _, err = admin.Exec("DROP DATABASE IF EXISTS " + schema); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec("CREATE DATABASE " + schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec("DROP DATABASE IF EXISTS " + schema)

	db, err := sql.Open("mysql", dsnWithSchema(dsn, schema))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err = db.Exec(`CREATE TABLE message_read_outbox (
  id BIGINT NOT NULL AUTO_INCREMENT,
  user_id BIGINT NOT NULL,
  peer_dialog_id BIGINT NOT NULL,
  read_user_id BIGINT NOT NULL,
  read_outbox_max_id INT NOT NULL,
  read_outbox_max_date BIGINT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY user_id (user_id, peer_dialog_id, read_user_id, read_outbox_max_id)
)`); err != nil {
		t.Fatal(err)
	}

	const (
		owner      = int64(10)
		reader     = int64(20)
		peerDialog = int64(20)
		msgID      = int32(7)
	)
	// The bug: the checkpoint exists, but the date column was left at 0.
	if _, err = db.Exec(`INSERT INTO message_read_outbox (user_id, peer_dialog_id, read_user_id, read_outbox_max_id, read_outbox_max_date) VALUES (?, ?, ?, ?, 0)`,
		owner, peerDialog, reader, msgID); err != nil {
		t.Fatal(err)
	}
	got := queryReadDate(t, db, owner, peerDialog, reader, msgID)
	if got == 0 {
		t.Fatal("getOutboxReadDate timestamp is 0")
	}
	now := time.Now().Unix()
	if int64(got) < now-120 || int64(got) > now+120 {
		t.Fatalf("fallback read date %d is not the row write time", got)
	}

	const first = int64(1_700_000_000)
	if _, err = db.Exec(PositionalUpsert(), owner, peerDialog, reader, msgID, first); err != nil {
		t.Fatal(err)
	}
	if got = queryReadDate(t, db, owner, peerDialog, reader, msgID); int64(got) != first {
		t.Fatalf("backfill date = %d, want %d", got, first)
	}
	if _, err = db.Exec(PositionalUpsert(), owner, peerDialog, reader, msgID, first+100); err != nil {
		t.Fatal(err)
	}
	if got = queryReadDate(t, db, owner, peerDialog, reader, msgID); int64(got) != first {
		t.Fatalf("later read overwrote the original time: %d", got)
	}
}

func queryReadDate(t *testing.T, db *sql.DB, owner, peerDialog, reader int64, msgID int32) int32 {
	t.Helper()
	var userID, raw int64
	err := db.QueryRow(SelectHitsSQL(true, true), owner, peerDialog, msgID, reader).Scan(&userID, &raw)
	if err != nil {
		t.Fatal(err)
	}
	if userID != reader {
		t.Fatalf("reader = %d, want %d", userID, reader)
	}
	got := Pick(raw)
	if got == 0 {
		t.Fatalf("getOutboxReadDate timestamp is 0 (raw %d)", raw)
	}
	return got
}

func dsnWithSchema(dsn, schema string) string {
	slash := strings.LastIndex(dsn, "/")
	if slash < 0 {
		return dsn + schema
	}
	rest := dsn[slash+1:]
	if q := strings.Index(rest, "?"); q >= 0 {
		return dsn[:slash+1] + schema + rest[q:]
	}
	return dsn[:slash+1] + schema
}
