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

func TestSavedMessagesTopMigration(t *testing.T) {
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

	const schema = "teamgram_saved_top"
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
  date2 BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, peer_type, peer_id)
)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`
CREATE TABLE messages (
  user_id BIGINT NOT NULL,
  user_message_box_id INT NOT NULL,
  peer_type INT NOT NULL,
  peer_id BIGINT NOT NULL,
  date2 BIGINT NOT NULL DEFAULT 0,
  deleted TINYINT NOT NULL DEFAULT 0
)`); err != nil {
		t.Fatal(err)
	}

	if _, err = db.Exec(`INSERT INTO dialogs (user_id, peer_type, peer_id, top_message, date2) VALUES
		(1, 2, 1, 0, 0),
		(2, 1, 2, 0, 0),
		(3, 2, 4, 1, 10),
		(4, 2, 4, 8, 80),
		(5, 2, 5, 12, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO messages (user_id, user_message_box_id, peer_type, peer_id, date2, deleted) VALUES
		(1, 4, 2, 1, 100, 0),
		(1, 9, 2, 1, 200, 0),
		(2, 3, 2, 2, 50, 0),
		(3, 8, 2, 4, 90, 0),
		(5, 12, 2, 5, 120, 1),
		(5, 6, 2, 5, 60, 0)`); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile("migrate-20261029-saved-messages-top.sql")
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

	want := map[[2]int64][2]int64{
		{1, 1}: {9, 200},
		{2, 2}: {3, 50},
		{3, 4}: {1, 10},
		{4, 4}: {8, 80},
		{5, 5}: {6, 60},
	}
	for key, exp := range want {
		var top, date int64
		err = db.QueryRow(`SELECT top_message, date2 FROM dialogs WHERE user_id=? AND peer_id=?`, key[0], key[1]).Scan(&top, &date)
		if err != nil {
			t.Fatalf("dialog %v: %v", key, err)
		}
		if top != exp[0] || date != exp[1] {
			t.Fatalf("dialog %v top=%d date=%d, want %d %d", key, top, date, exp[0], exp[1])
		}
	}
}
