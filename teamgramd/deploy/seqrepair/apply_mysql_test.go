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

package seqrepair

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

// The same fixture as TestApplyTwiceLeavesEditsDeletesAndContinuation, written
// through Load/Run so a second -apply and a following dry-run both report
// nothing. Skips when MySQL is not reachable; the in-memory test always runs.
func TestApplyTwiceOnMySQL(t *testing.T) {
	const (
		alice = int64(1001)
		bob   = int64(1002)
	)
	db := openSeqrepairDB(t)
	ctx := context.Background()
	if err := insertFixture(ctx, db, idempotentFixture(alice, bob)); err != nil {
		t.Fatal(err)
	}
	seq := NewMemSeq(nil)

	first, err := Run(ctx, db, seq, true)
	if err != nil {
		t.Fatal(err)
	}
	if first.Plan.Rebuilt != 1 || first.Plan.PtsMoved != 5 {
		t.Fatalf("first apply rebuilt %d pts %d, want 1 and 5\n%s", first.Plan.Rebuilt, first.Plan.PtsMoved, first.Format("applied"))
	}
	if first.DupPairs != 0 {
		t.Fatalf("duplicates after apply: %+v", first.Dups)
	}
	applied := first.Format("applied")
	if strings.Contains(applied, "大郎请喝药\"") || strings.Contains(applied, "@mke123456\"") || strings.Contains(applied, "gone") {
		t.Fatalf("first apply restored an edit or a delete:\n%s", applied)
	}
	if !strings.Contains(applied, "lost-one") {
		t.Fatalf("first apply did not restore the lost message:\n%s", applied)
	}

	n, err := countMessages(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if n != 7 {
		t.Fatalf("message rows = %d, want 6 stored + 1 restored", n)
	}
	assertTextCount(t, db, "大郎请喝药", 1)
	assertTextCount(t, db, "大郎请喝药 1", 1)
	assertTextCount(t, db, "@mke123456", 0)
	assertTextCount(t, db, "@mke123456 2", 1)
	assertTextCount(t, db, "gone", 1)
	var goneDeleted int
	if err = db.QueryRow(`SELECT deleted FROM messages WHERE message=?`, "gone").Scan(&goneDeleted); err != nil {
		t.Fatal(err)
	}
	if goneDeleted == 0 {
		t.Fatal("deleted message was undeleted")
	}
	var cont41, cont42 int
	if err = db.QueryRow(`SELECT pts FROM user_pts_updates WHERE id=9`).Scan(&cont41); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT pts FROM user_pts_updates WHERE id=10`).Scan(&cont42); err != nil {
		t.Fatal(err)
	}
	if cont41 != 41 || cont42 != 42 {
		t.Fatalf("continuation pts = %d,%d, want 41,42", cont41, cont42)
	}

	dry, err := Run(ctx, db, seq, false)
	if err != nil {
		t.Fatal(err)
	}
	dryText := dry.Format("dry-run")
	if !strings.Contains(dryText, "nothing to repair") {
		t.Fatalf("dry-run after apply:\n%s", dryText)
	}
	second, err := Run(ctx, db, seq, true)
	if err != nil {
		t.Fatal(err)
	}
	secondText := second.Format("applied")
	if !strings.Contains(secondText, "nothing to repair") {
		t.Fatalf("second apply:\n%s", secondText)
	}
	n2, err := countMessages(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if n2 != n {
		t.Fatalf("second apply inserted messages: %d -> %d", n, n2)
	}
}

func openSeqrepairDB(t *testing.T) *sql.DB {
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
	const schema = "teamgram_seqrepair"
	if _, err = admin.Exec("DROP DATABASE IF EXISTS " + schema); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec("CREATE DATABASE " + schema + " CHARACTER SET utf8mb4"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS " + schema) })

	db, err := sql.Open("mysql", seqrepairDSN(dsn, schema))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, stmt := range strings.Split(seqrepairSchema, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err = db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

const seqrepairSchema = `
CREATE TABLE messages (
  id BIGINT NOT NULL AUTO_INCREMENT,
  user_id BIGINT NOT NULL,
  user_message_box_id INT NOT NULL,
  dialog_id1 BIGINT NOT NULL DEFAULT 0,
  dialog_id2 BIGINT NOT NULL DEFAULT 0,
  dialog_message_id BIGINT NOT NULL DEFAULT 0,
  sender_user_id BIGINT NOT NULL,
  peer_type INT NOT NULL,
  peer_id BIGINT NOT NULL,
  message_data JSON NULL,
  message VARCHAR(6000) NOT NULL DEFAULT '',
  date2 BIGINT NOT NULL DEFAULT 0,
  deleted TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY user_box (user_id, user_message_box_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE user_pts_updates (
  id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  pts INT NOT NULL,
  pts_count INT NOT NULL,
  update_type TINYINT NOT NULL DEFAULT 0,
  update_data JSON NOT NULL,
  date2 BIGINT NOT NULL,
  PRIMARY KEY (id),
  KEY user_pts (user_id, pts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE dialogs (
  id BIGINT NOT NULL AUTO_INCREMENT,
  user_id BIGINT NOT NULL,
  peer_type INT NOT NULL,
  peer_id BIGINT NOT NULL,
  top_message INT NOT NULL DEFAULT 0,
  read_inbox_max_id INT NOT NULL DEFAULT 0,
  read_outbox_max_id INT NOT NULL DEFAULT 0,
  unread_count INT NOT NULL DEFAULT 0,
  date2 BIGINT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY user_peer (user_id, peer_type, peer_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
`

func insertFixture(ctx context.Context, db *sql.DB, snap Snapshot) error {
	for _, m := range snap.Messages {
		deleted := 0
		if m.Deleted {
			deleted = 1
		}
		if _, err := db.ExecContext(ctx, `
INSERT INTO messages
  (user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id,
   sender_user_id, peer_type, peer_id, message, date2, deleted)
VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			m.UserID, m.BoxID, m.DialogID1, m.DialogID2, m.DialogMessageID,
			m.SenderID, m.PeerType, m.PeerID, m.Message, m.Date2, deleted); err != nil {
			return err
		}
	}
	for _, row := range snap.Pts {
		if _, err := db.ExecContext(ctx, `
INSERT INTO user_pts_updates (id, user_id, pts, pts_count, update_type, update_data, date2)
VALUES (?,?,?,?,?,?,?)`,
			row.ID, row.UserID, row.Pts, row.PtsCount, row.UpdateType, row.UpdateData, row.Date2); err != nil {
			return err
		}
	}
	for _, d := range snap.Dialogs {
		if _, err := db.ExecContext(ctx, `
INSERT INTO dialogs
  (user_id, peer_type, peer_id, top_message, read_inbox_max_id, read_outbox_max_id, unread_count, date2)
VALUES (?,?,?,?,?,?,?,?)`,
			d.UserID, d.PeerType, d.PeerID, d.TopMessage, d.ReadInbox, d.ReadOutbox, d.UnreadCount, d.Date2); err != nil {
			return err
		}
	}
	return nil
}

func countMessages(ctx context.Context, db *sql.DB) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages`).Scan(&n)
	return n, err
}

func assertTextCount(t *testing.T, db *sql.DB, text string, want int) {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM messages WHERE message=?`, text).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != want {
		t.Fatalf("message %q count = %d, want %d", text, n, want)
	}
}

func seqrepairDSN(dsn, schema string) string {
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
