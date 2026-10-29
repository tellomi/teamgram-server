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
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

func TestReadOutboxCleanup(t *testing.T) {
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

	const schema = "teamgram_read_outbox_cleanup"
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
	if err = db.Ping(); err != nil {
		t.Fatal(err)
	}

	if _, err = db.Exec(`
CREATE TABLE dialogs (
  user_id BIGINT NOT NULL,
  peer_type INT NOT NULL,
  peer_id BIGINT NOT NULL,
  top_message INT NOT NULL DEFAULT 0,
  pinned BIGINT NOT NULL DEFAULT 0,
  folder_pinned BIGINT NOT NULL DEFAULT 0,
  draft_type INT NOT NULL DEFAULT 0,
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
  dialog_message_id BIGINT NOT NULL,
  sender_user_id BIGINT NOT NULL,
  peer_type INT NOT NULL,
  peer_id BIGINT NOT NULL,
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

	dialogs := []struct {
		user, peer int64
		peerType   int
		top, draft int
		in, out    int
	}{
		{10, 10, 2, 0, 0, 0, 50},  // fake self dialog
		{11, 11, 2, 3, 0, 0, 99},  // Saved Messages with history
		{12, 12, 1, 0, 0, 0, 5},   // Saved Messages stored as peerSelf
		{13, 13, 2, 0, 2, 0, 7},   // Saved Messages that only has a draft
		{20, 21, 2, 30, 0, 0, 30}, // private outbox marked past what the peer read
		{20, 22, 2, 8, 0, 0, 8},   // private outbox already correct
		{21, 20, 2, 8, 0, 4, 0},
		{22, 20, 2, 3, 0, 3, 0},
		{20, 100, 3, 60, 0, 50, 40}, // group outbox marked by the sender's own read
		{30, 100, 3, 12, 0, 0, 12},  // group outbox someone else has read
		{40, 100, 3, 9, 0, 0, 9},    // only the older outgoing message was read
		{100, 100, 3, 0, 0, 0, 1},   // chat id happens to equal the user id
		{50, 51, 2, 0, 0, 0, 15},    // receipt lives only in message_read_outbox
		{60, 61, 2, 6, 0, 0, 2},     // do not raise a marker that is behind
		{61, 60, 2, 6, 0, 6, 0},
	}
	for _, d := range dialogs {
		if _, err = db.Exec(`INSERT INTO dialogs (user_id, peer_type, peer_id, top_message, draft_type, read_inbox_max_id, read_outbox_max_id) VALUES (?,?,?,?,?,?,?)`,
			d.user, d.peerType, d.peer, d.top, d.draft, d.in, d.out); err != nil {
			t.Fatal(err)
		}
	}

	messages := []struct {
		user, sender, peer int64
		peerType, box      int
		dataID             int64
	}{
		{11, 11, 11, 2, 3, 900},
		{20, 20, 21, 2, 10, 501},
		{21, 20, 20, 2, 4, 501},
		{20, 20, 21, 2, 30, 502},
		{21, 20, 20, 2, 8, 502},
		{20, 20, 22, 2, 8, 601},
		{22, 20, 20, 2, 3, 601},
		{20, 20, 100, 3, 40, 701},
		{30, 20, 100, 3, 7, 701},
		{30, 30, 100, 3, 12, 702},
		{20, 30, 100, 3, 41, 702},
		{40, 40, 100, 3, 5, 801},
		{20, 40, 100, 3, 50, 801},
		{40, 40, 100, 3, 9, 802},
		{20, 40, 100, 3, 60, 802},
		{60, 60, 61, 2, 2, 901},
		{61, 60, 60, 2, 2, 901},
		{60, 60, 61, 2, 6, 902},
		{61, 60, 60, 2, 6, 902},
	}
	for _, m := range messages {
		if _, err = db.Exec(`INSERT INTO messages (user_id, user_message_box_id, dialog_message_id, sender_user_id, peer_type, peer_id, deleted) VALUES (?,?,?,?,?,?,0)`,
			m.user, m.box, m.dataID, m.sender, m.peerType, m.peer); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`INSERT INTO message_read_outbox (user_id, peer_dialog_id, read_user_id, read_outbox_max_id) VALUES (20,20,20,30),(20,21,21,10),(50,51,51,15)`); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile("migrate-20261029-read-outbox.sql")
	if err != nil {
		t.Fatal(err)
	}
	apply := func() {
		t.Helper()
		for _, stmt := range sqlStatements(string(raw)) {
			if _, err = db.Exec(stmt); err != nil {
				t.Fatalf("%s: %v", stmt, err)
			}
		}
	}
	apply()
	apply()

	if dialogExists(t, db, 10, 2, 10) {
		t.Fatal("fake self dialog was kept")
	}
	for _, kept := range [][3]int64{{11, 2, 11}, {12, 1, 12}, {13, 2, 13}, {100, 3, 100}} {
		if !dialogExists(t, db, kept[0], int(kept[1]), kept[2]) {
			t.Fatalf("dialog %v was deleted", kept)
		}
	}

	wantOutbox := map[[3]int64]int{
		{11, 2, 11}:   0,
		{12, 1, 12}:   0,
		{13, 2, 13}:   0,
		{20, 2, 21}:   10,
		{20, 2, 22}:   8,
		{20, 3, 100}:  0,
		{30, 3, 100}:  12,
		{40, 3, 100}:  5,
		{100, 3, 100}: 0,
		{50, 2, 51}:   15,
		{60, 2, 61}:   2,
	}
	for key, want := range wantOutbox {
		var got int
		err = db.QueryRow(`SELECT read_outbox_max_id FROM dialogs WHERE user_id=? AND peer_type=? AND peer_id=?`, key[0], key[1], key[2]).Scan(&got)
		if err != nil {
			t.Fatalf("dialog %v: %v", key, err)
		}
		if got != want {
			t.Fatalf("dialog %v read_outbox_max_id=%d, want %d", key, got, want)
		}
	}

	var selfReads int
	if err = db.QueryRow(`SELECT COUNT(*) FROM message_read_outbox WHERE user_id=read_user_id`).Scan(&selfReads); err != nil {
		t.Fatal(err)
	}
	if selfReads != 0 {
		t.Fatalf("self read rows left: %d", selfReads)
	}
	var peerReads int
	if err = db.QueryRow(`SELECT COUNT(*) FROM message_read_outbox WHERE user_id<>read_user_id`).Scan(&peerReads); err != nil {
		t.Fatal(err)
	}
	if peerReads != 2 {
		t.Fatalf("peer read rows = %d, want 2", peerReads)
	}
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

func dialogExists(t *testing.T, db *sql.DB, user int64, peerType int, peer int64) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dialogs WHERE user_id=? AND peer_type=? AND peer_id=?`, user, peerType, peer).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func sqlStatements(file string) []string {
	var b strings.Builder
	for _, line := range strings.Split(file, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	var out []string
	for _, part := range strings.Split(b.String(), ";") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
