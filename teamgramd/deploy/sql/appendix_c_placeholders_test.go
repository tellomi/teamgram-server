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
	"strings"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/usernamepolicy"
)

func TestAppendixCPlaceholdersMySQL(t *testing.T) {
	db := openTellomiMySQL(t)
	createUsernameTable(t, db)
	installFailHelper(t, db)

	applyAppendixC(t, db)
	assertAppendixCPlaceholders(t, db)
	applyAppendixC(t, db)
	assertAppendixCPlaceholders(t, db)

	if _, err := db.Exec(`INSERT INTO username (username, peer_type, peer_id, deleted) VALUES ('WECHAT', 2, 1, 0)`); err == nil {
		t.Fatal("WECHAT insert succeeded; utf8mb4_unicode_ci should reject it")
	}
	if _, err := db.Exec(`INSERT INTO username (username, peer_type, peer_id, deleted) VALUES ('Google', 2, 1, 0)`); err == nil {
		t.Fatal("Google insert succeeded; utf8mb4_unicode_ci should reject it")
	}
}

func TestAppendixCPlaceholdersMySQLDoesNotSteal(t *testing.T) {
	db := openTellomiMySQL(t)
	createUsernameTable(t, db)
	installFailHelper(t, db)
	if _, err := db.Exec(`INSERT INTO username (username, peer_type, peer_id, editable, active, order2, deleted) VALUES ('TELLOMI', 2, 7, 1, 1, 0, 0), ('wechat', 2, 42, 1, 1, 0, 0)`); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(mustReadMigration(t, "migrate-20261109-appendix-c-placeholders.sql"))
	if err == nil || !strings.Contains(err.Error(), "D-56: reserved username taken:") {
		t.Fatalf("migration error = %v, want D-56 refusal", err)
	}
	var peer int64
	if err := db.QueryRow(`SELECT peer_id FROM username WHERE username = 'wechat'`).Scan(&peer); err != nil {
		t.Fatal(err)
	}
	if peer != 42 {
		t.Fatalf("wechat peer = %d", peer)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM username WHERE username = 'google'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("migration inserted google after refusing wechat")
	}
	if err := db.QueryRow(`SELECT peer_id FROM username WHERE username = 'tellomi'`).Scan(&peer); err != nil {
		t.Fatal(err)
	}
	if peer != 7 {
		t.Fatalf("tellomi peer = %d, placeholder migration must not touch it", peer)
	}
}

func applyAppendixC(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(mustReadMigration(t, "migrate-20261109-appendix-c-placeholders.sql")); err != nil {
		t.Fatal(err)
	}
}

func assertAppendixCPlaceholders(t *testing.T, db *sql.DB) {
	t.Helper()
	occupied, unresolved := usernamepolicy.AppendixCPlaceholders()
	for _, name := range occupied {
		for _, lookup := range []string{name, strings.ToUpper(name)} {
			var peerType int
			var peerID int64
			var editable, active, deleted int
			err := db.QueryRow(
				`SELECT peer_type, peer_id, editable, active, deleted FROM username WHERE username = ?`,
				lookup,
			).Scan(&peerType, &peerID, &editable, &active, &deleted)
			if err != nil {
				t.Fatalf("lookup %s: %v", lookup, err)
			}
			// peer_type 0 is the empty placeholder. user.resolveUsername
			// returns USERNAME_NOT_OCCUPIED for it. account.checkUsername
			// still sees the row and returns USERNAME_OCCUPIED.
			if peerType != 0 || peerID != 0 || editable != 0 || active != 0 || deleted != 0 {
				t.Fatalf("%s => type %d peer %d editable %d active %d deleted %d", lookup, peerType, peerID, editable, active, deleted)
			}
		}
	}
	for _, name := range unresolved {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM username WHERE username = ?`, name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("short name %s has a row", name)
		}
	}
}
