// Copyright 2026 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package cleanup

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

func TestReadOutboxRecompute(t *testing.T) {
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

	const schema = "teamgram_read_outbox_recompute"
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

	if _, err = db.Exec(`
CREATE TABLE dialogs (
  user_id BIGINT NOT NULL,
  peer_type INT NOT NULL,
  peer_id BIGINT NOT NULL,
  top_message INT NOT NULL DEFAULT 0,
  read_inbox_max_id INT NOT NULL DEFAULT 0,
  read_outbox_max_id INT NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, peer_type, peer_id)
)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`
CREATE TABLE messages (
  user_id BIGINT NOT NULL,
  user_message_box_id INT NOT NULL,
  sender_user_id BIGINT NOT NULL,
  peer_type INT NOT NULL,
  peer_id BIGINT NOT NULL,
  dialog_message_id BIGINT NOT NULL,
  deleted TINYINT NOT NULL DEFAULT 0
)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`
CREATE TABLE message_read_outbox (
  user_id BIGINT NOT NULL,
  peer_dialog_id BIGINT NOT NULL,
  read_user_id BIGINT NOT NULL,
  read_outbox_max_id INT NOT NULL
)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`
CREATE TABLE channel_messages (
  channel_id BIGINT NOT NULL,
  message_id INT NOT NULL,
  sender_user_id BIGINT NOT NULL,
  deleted TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (channel_id, message_id)
)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`
CREATE TABLE channel_read_state (
  channel_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  read_inbox_max_id INT NOT NULL DEFAULT 0,
  read_outbox_max_id INT NOT NULL DEFAULT 0,
  unread_count INT NOT NULL DEFAULT 0,
  PRIMARY KEY (channel_id, user_id)
)`); err != nil {
		t.Fatal(err)
	}

	// Private: user 1's message 9 is unread by user 2 (inbox 4). The stored
	// outbox of 9 is the self-read bug. User 2 has read 1's message 3.
	if _, err = db.Exec(`
INSERT INTO dialogs (user_id, peer_type, peer_id, top_message, read_inbox_max_id, read_outbox_max_id) VALUES
  (1, 2, 2, 9, 4, 9),
  (2, 2, 1, 8, 4, 0),
  (1, 1, 1, 2, 2, 2),
  (1, 4, 50, 6, 6, 6)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`
INSERT INTO messages (user_id, user_message_box_id, sender_user_id, peer_type, peer_id, dialog_message_id, deleted) VALUES
  (1, 3, 1, 2, 2, 100, 0),
  (2, 4, 1, 2, 1, 100, 0),
  (1, 9, 1, 2, 2, 200, 0),
  (2, 8, 1, 2, 1, 200, 0)`); err != nil {
		t.Fatal(err)
	}
	// Supergroup 50. User 1 sent 5 and 6 and has inbox 6 because they sent 6,
	// which also covers user 2's older message 4. User 2 has read through 5,
	// so user 1's message 6 is still unread. Both outboxes were set to 6 on send.
	if _, err = db.Exec(`
INSERT INTO channel_messages (channel_id, message_id, sender_user_id, deleted) VALUES
  (50, 5, 1, 0),
  (50, 6, 1, 0),
  (50, 4, 2, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`
INSERT INTO channel_read_state (channel_id, user_id, read_inbox_max_id, read_outbox_max_id, unread_count) VALUES
  (50, 1, 6, 6, 0),
  (50, 2, 5, 6, 1),
  (50, 3, 0, 6, 3)`); err != nil {
		t.Fatal(err)
	}

	apply := func() {
		t.Helper()
		raw, err := os.ReadFile("migrate-20261030-read-outbox.sql")
		if err != nil {
			t.Fatal(err)
		}
		for _, stmt := range sqlStatements(string(raw)) {
			if _, err = db.Exec(stmt); err != nil {
				t.Fatalf("sql: %v\n%s", err, stmt)
			}
		}
	}
	apply()
	apply()

	got := func(table, where string, args ...any) int {
		t.Helper()
		var n int
		q := "SELECT read_outbox_max_id FROM " + table + " WHERE " + where
		if err := db.QueryRow(q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if g := got("dialogs", "user_id=? AND peer_type=? AND peer_id=?", 1, 2, 2); g != 3 {
		t.Fatalf("private outbox = %d, want 3", g)
	}
	if g := got("dialogs", "user_id=? AND peer_type=? AND peer_id=?", 1, 1, 1); g != 0 {
		t.Fatalf("saved messages outbox = %d, want 0", g)
	}
	if g := got("dialogs", "user_id=? AND peer_type=? AND peer_id=?", 1, 4, 50); g != 5 {
		t.Fatalf("channel dialog shadow outbox = %d, want 5", g)
	}
	if g := got("channel_read_state", "channel_id=? AND user_id=?", 50, 1); g != 5 {
		t.Fatalf("author outbox = %d, want 5 (member 2 read through 5, not the unread 6)", g)
	}
	if g := got("channel_read_state", "channel_id=? AND user_id=?", 50, 2); g != 4 {
		t.Fatalf("member 2 outbox = %d, want 4 (user 1's inbox covers message 4, not the false 6)", g)
	}
	if g := got("channel_read_state", "channel_id=? AND user_id=?", 50, 3); g != 0 {
		t.Fatalf("member 3 outbox = %d, want 0", g)
	}

	var backed int
	if err = db.QueryRow(`SELECT read_outbox_max_id FROM dialogs_read_outbox_bak_20261030 WHERE user_id=1 AND peer_type=2 AND peer_id=2`).Scan(&backed); err != nil {
		t.Fatal(err)
	}
	if backed != 9 {
		t.Fatalf("backup was overwritten: %d, want the pre-cleanup value 9", backed)
	}
}
