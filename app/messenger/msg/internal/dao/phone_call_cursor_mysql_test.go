package dao

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

// TestAnsweredCallCursorKeepsEarlierUnread runs the same UPDATE the inbox
// uses. A caught-up dialog moves the cursor onto the call and stays at
// unread 0. A dialog that still has an incoming message before the call
// keeps that cursor and that unread count. The caller's dialog is a
// different user_id and is not updated.
func TestAnsweredCallCursorKeepsEarlierUnread(t *testing.T) {
	dsn := os.Getenv("TEAMGRAM_MYSQL_DSN")
	if dsn == "" {
		dsn = "root:@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true&multiStatements=true"
	}
	admin, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("mysql not available: %v", err)
	}
	defer admin.Close()
	admin.SetMaxOpenConns(1)
	if err = admin.Ping(); err != nil {
		t.Skipf("mysql not available: %v", err)
	}

	const schema = "teamgram_answered_call_read"
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
	if _, err = db.Exec(`CREATE TABLE messages (
		user_id BIGINT NOT NULL,
		user_message_box_id INT NOT NULL,
		peer_type INT NOT NULL,
		peer_id BIGINT NOT NULL,
		sender_user_id BIGINT NOT NULL,
		deleted TINYINT NOT NULL DEFAULT 0
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE dialogs (
		user_id BIGINT NOT NULL,
		peer_type INT NOT NULL,
		peer_id BIGINT NOT NULL,
		top_message INT NOT NULL,
		read_inbox_max_id INT NOT NULL,
		unread_count INT NOT NULL,
		unread_mark TINYINT NOT NULL DEFAULT 0,
		PRIMARY KEY (user_id, peer_type, peer_id)
	)`); err != nil {
		t.Fatal(err)
	}

	const (
		callee = int64(2)
		caller = int64(1)
		peer   = mtprotoPeerUser
	)
	// Callee has unread message 10, then an answered call at 12.
	// Message 11 is the callee's own outgoing row and must not count.
	// The caller has unread message 8 from the callee; that dialog must stay.
	if _, err = db.Exec(`INSERT INTO messages (user_id, user_message_box_id, peer_type, peer_id, sender_user_id, deleted) VALUES
		(?, 10, ?, ?, ?, 0),
		(?, 11, ?, ?, ?, 0),
		(?, 12, ?, ?, ?, 0),
		(?, 8, ?, ?, ?, 0)`,
		callee, peer, caller, caller,
		callee, peer, caller, callee,
		callee, peer, caller, caller,
		caller, peer, callee, callee,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO dialogs (user_id, peer_type, peer_id, top_message, read_inbox_max_id, unread_count) VALUES
		(?, ?, ?, 12, 9, 1),
		(?, ?, ?, 8, 4, 1)`,
		callee, peer, caller,
		caller, peer, callee,
	); err != nil {
		t.Fatal(err)
	}

	res, err := db.Exec(answeredCallCursorSQL("messages"),
		int32(12), int32(12), int32(12),
		callee, peer, caller,
		int32(12),
		callee, peer, caller, callee, int32(12),
	)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n != 0 {
		t.Fatalf("cursor moved across an unread message, rows %d", n)
	}
	var readInbox, unread int32
	if err = db.QueryRow(`SELECT read_inbox_max_id, unread_count FROM dialogs WHERE user_id=? AND peer_id=?`, callee, caller).Scan(&readInbox, &unread); err != nil {
		t.Fatal(err)
	}
	if readInbox != 9 || unread != 1 {
		t.Fatalf("callee dialog read=%d unread=%d, want read 9 unread 1", readInbox, unread)
	}
	if err = db.QueryRow(`SELECT read_inbox_max_id, unread_count FROM dialogs WHERE user_id=? AND peer_id=?`, caller, callee).Scan(&readInbox, &unread); err != nil {
		t.Fatal(err)
	}
	if readInbox != 4 || unread != 1 {
		t.Fatalf("caller dialog read=%d unread=%d, want them unchanged", readInbox, unread)
	}

	// Caught up: cursor already covers message 10. The call at 12 is the top.
	if _, err = db.Exec(`UPDATE messages SET deleted=1 WHERE user_id=? AND user_message_box_id=10`, callee); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE dialogs SET read_inbox_max_id=11, unread_count=0 WHERE user_id=? AND peer_id=?`, callee, caller); err != nil {
		t.Fatal(err)
	}
	res, err = db.Exec(answeredCallCursorSQL("messages"),
		int32(12), int32(12), int32(12),
		callee, peer, caller,
		int32(12),
		callee, peer, caller, callee, int32(12),
	)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		t.Fatal("caught-up dialog did not move the cursor onto the answered call")
	}
	if err = db.QueryRow(`SELECT read_inbox_max_id, unread_count FROM dialogs WHERE user_id=? AND peer_id=?`, callee, caller).Scan(&readInbox, &unread); err != nil {
		t.Fatal(err)
	}
	if readInbox != 12 || unread != 0 {
		t.Fatalf("caught-up callee read=%d unread=%d, want 12 and 0", readInbox, unread)
	}
}

const mtprotoPeerUser = 2

func dsnWithSchema(dsn, schema string) string {
	slash := -1
	for i := 0; i < len(dsn); i++ {
		if dsn[i] == '/' {
			slash = i
		}
	}
	if slash < 0 {
		return dsn
	}
	end := slash + 1
	for end < len(dsn) && dsn[end] != '?' {
		end++
	}
	return dsn[:slash+1] + schema + dsn[end:]
}
