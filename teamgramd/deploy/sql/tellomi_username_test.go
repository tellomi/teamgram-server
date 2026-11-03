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

func TestTellomiUsernameMigrationMySQL(t *testing.T) {
	db := openTellomiMySQL(t)
	createUsernameTable(t, db)
	installFailHelper(t, db)

	applyTellomiUsernameMigration(t, db)
	assertTellomiOwnedBy(t, db, 777000)
	applyTellomiUsernameMigration(t, db)
	assertTellomiOwnedBy(t, db, 777000)

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM username WHERE username = 'tellomi'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("username rows = %d, want 1", n)
	}
	if _, err := db.Exec(`INSERT INTO username (username, peer_type, peer_id, deleted) VALUES ('Tellomi', 2, 1, 0)`); err == nil {
		t.Fatal("Tellomi insert succeeded; utf8mb4_unicode_ci should reject it")
	}
	if _, err := db.Exec(`INSERT INTO username (username, peer_type, peer_id, deleted) VALUES ('TELLOMI', 2, 1, 0)`); err == nil {
		t.Fatal("TELLOMI insert succeeded; utf8mb4_unicode_ci should reject it")
	}
}

func TestTellomiUsernameMigrationDoesNotSteal(t *testing.T) {
	db := openTellomiMySQL(t)
	createUsernameTable(t, db)
	installFailHelper(t, db)
	if _, err := db.Exec(`INSERT INTO username (username, peer_type, peer_id, editable, active, order2, deleted) VALUES ('tellomi', 2, 42, 1, 1, 0, 0)`); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(mustReadMigration(t, "migrate-20261104-tellomi-username.sql"))
	if err == nil || !strings.Contains(err.Error(), "D-27:") {
		t.Fatalf("migration error = %v, want D-27 refusal", err)
	}
	assertTellomiOwnedBy(t, db, 42)
}

func TestTellomiUsernameMigrationLeavesAIUser(t *testing.T) {
	db := openTellomiMySQL(t)
	createUsernameTable(t, db)
	installFailHelper(t, db)
	if _, err := db.Exec(`INSERT INTO username (username, peer_type, peer_id, editable, active, order2, deleted) VALUES ('tellomi', 2, 777001, 1, 1, 2, 0)`); err != nil {
		t.Fatal(err)
	}
	applyTellomiUsernameMigration(t, db)
	var peer int64
	if err := db.QueryRow(`SELECT peer_id FROM username WHERE username = 'tellomi'`).Scan(&peer); err != nil {
		t.Fatal(err)
	}
	if peer != 777001 {
		t.Fatalf("peer = %d, want 777001 left in place", peer)
	}
}

func installFailHelper(t *testing.T, db *sql.DB) {
	t.Helper()
	var name string
	if err := db.QueryRow("SELECT DATABASE()").Scan(&name); err != nil {
		t.Fatal(err)
	}
	dsn := os.Getenv("TEAMGRAM_MYSQL_DSN")
	if dsn == "" {
		dsn = "teamgram:teamgram@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true"
	}
	dsn = strings.ReplaceAll(dsn, "multiStatements=true", "")
	dsn = strings.ReplaceAll(dsn, "&&", "&")
	dsn = strings.TrimRight(dsn, "?&")
	slash := strings.LastIndex(dsn, "/")
	if slash < 0 {
		t.Fatalf("dsn has no database: %s", dsn)
	}
	rest := dsn[slash+1:]
	query := ""
	if q := strings.Index(rest, "?"); q >= 0 {
		query = rest[q:]
	}
	plain, err := sql.Open("mysql", dsn[:slash+1]+name+query)
	if err != nil {
		t.Fatal(err)
	}
	defer plain.Close()
	if _, err = plain.Exec("DROP PROCEDURE IF EXISTS teamgram_fail_if_set"); err != nil {
		t.Fatal(err)
	}
	_, err = plain.Exec(`CREATE PROCEDURE teamgram_fail_if_set(IN detail VARCHAR(128), IN prefix VARCHAR(128))
BEGIN
  IF detail IS NOT NULL THEN
    SET @teamgram_fail = CONCAT(prefix, detail);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = @teamgram_fail;
  END IF;
END`)
	if err != nil {
		t.Fatal(err)
	}
}

func openTellomiMySQL(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEAMGRAM_MYSQL_DSN")
	if dsn == "" {
		dsn = "teamgram:teamgram@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true&multiStatements=true"
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("mysql not available: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err = db.Ping(); err != nil {
		t.Skipf("mysql not available: %v", err)
	}
	name := "tellomi_d27_" + stringsForTest(t)
	if _, err = db.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP DATABASE " + name) })
	if _, err = db.Exec("USE " + name); err != nil {
		t.Fatal(err)
	}
	return db
}

func stringsForTest(t *testing.T) string {
	t.Helper()
	s := t.Name()
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return "t"
	}
	return string(out)
}

func createUsernameTable(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
CREATE TABLE username (
  id bigint NOT NULL AUTO_INCREMENT,
  username varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  peer_type int NOT NULL DEFAULT 0,
  peer_id bigint NOT NULL DEFAULT 0,
  editable tinyint(1) NOT NULL DEFAULT 1,
  active tinyint(1) NOT NULL DEFAULT 1,
  order2 bigint NOT NULL DEFAULT 0,
  deleted tinyint(1) NOT NULL DEFAULT 0,
  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`)
	if err != nil {
		t.Fatal(err)
	}
}

func applyTellomiUsernameMigration(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(mustReadMigration(t, "migrate-20261104-tellomi-username.sql")); err != nil {
		t.Fatal(err)
	}
}

func mustReadMigration(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func assertTellomiOwnedBy(t *testing.T, db *sql.DB, peerID int64) {
	t.Helper()
	for _, name := range []string{"tellomi", "Tellomi", "TELLOMI"} {
		var gotType int
		var gotID int64
		var editable, active, deleted int
		err := db.QueryRow(
			`SELECT peer_type, peer_id, editable, active, deleted FROM username WHERE username = ?`,
			name,
		).Scan(&gotType, &gotID, &editable, &active, &deleted)
		if err != nil {
			t.Fatalf("lookup %s: %v", name, err)
		}
		if gotType != 2 || gotID != peerID || editable != 1 || active != 1 || deleted != 0 {
			t.Fatalf("%s => type %d peer %d editable %d active %d deleted %d", name, gotType, gotID, editable, active, deleted)
		}
	}
}
