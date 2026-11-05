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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/teamgram/teamgram-server/pkg/usernamepolicy"
)

type officialName struct {
	name     string
	peerType int
	peerID   int64
	editable int
	active   int
}

func TestOfficialAccountsMySQL(t *testing.T) {
	db := newSchemaDB(t, "tellomi_official_happy")
	applyTeamgramSchema(t, db, "tellomi_official_happy", true)
	assertOfficialAccounts(t, db)
	if err := mysqlFile(t, "tellomi_official_happy", "migrate-20261105-official-accounts.sql"); err != nil {
		t.Fatal(err)
	}
	if err := mysqlFile(t, "tellomi_official_happy", "migrate-20261109-appendix-c-placeholders.sql"); err != nil {
		t.Fatal(err)
	}
	assertOfficialAccounts(t, db)
	if _, err := db.Exec(`INSERT INTO username (username, peer_type, peer_id, deleted) VALUES ('TELLOMI', 2, 1, 0)`); err == nil {
		t.Fatal("TELLOMI insert succeeded")
	}
	if _, err := db.Exec(`INSERT INTO username (username, peer_type, peer_id, deleted) VALUES ('WECHAT', 2, 1, 0)`); err == nil {
		t.Fatal("WECHAT insert succeeded")
	}
}

func TestTellomiAINameAndResolveTarget(t *testing.T) {
	db := newSchemaDB(t, "tellomi_ai_name")
	applyTeamgramSchema(t, db, "tellomi_ai_name", true)
	assertTellomiAI(t, db)

	if _, err := db.Exec(`UPDATE users SET username = 'parked', first_name = 'Tellomi', about = '' WHERE id = 777001`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE users SET username = 'tellomi', first_name = 'Teamgram' WHERE id = 777000`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE username SET peer_id = 777000 WHERE username = 'tellomi'`); err != nil {
		t.Fatal(err)
	}
	if err := mysqlFile(t, "tellomi_ai_name", "migrate-20261107-tellomi-ai-name.sql"); err != nil {
		t.Fatal(err)
	}
	assertTellomiAI(t, db)
	if err := mysqlFile(t, "tellomi_ai_name", "migrate-20261107-tellomi-ai-name.sql"); err != nil {
		t.Fatal(err)
	}
	assertTellomiAI(t, db)
}

func assertTellomiAI(t *testing.T, db *sql.DB) {
	t.Helper()
	var first, about, name string
	if err := db.QueryRow(`SELECT first_name, about, username FROM users WHERE id = 777001`).Scan(&first, &about, &name); err != nil {
		t.Fatal(err)
	}
	if first != "Tellomi AI" || name != "tellomi" || about != "Tellomi 智能助手。直接发消息，即可获得解答、建议与灵感。" {
		t.Fatalf("777001 = %q %q %q", first, name, about)
	}
	var svcFirst, svcName string
	if err := db.QueryRow(`SELECT first_name, username FROM users WHERE id = 777000`).Scan(&svcFirst, &svcName); err != nil {
		t.Fatal(err)
	}
	if svcFirst != "Tellomi" || svcName != "admin" {
		t.Fatalf("777000 = %q %q", svcFirst, svcName)
	}
	var n int
	var peer int64
	var peerType int
	if err := db.QueryRow(`SELECT COUNT(*), MIN(peer_id), MIN(peer_type) FROM username WHERE username = 'tellomi'`).Scan(&n, &peer, &peerType); err != nil {
		t.Fatal(err)
	}
	if n != 1 || peer != 777001 || peerType != 2 {
		t.Fatalf("@tellomi rows = %d peer %d type %d", n, peer, peerType)
	}
}

func TestOfficialAccountsDoNotSteal(t *testing.T) {
	db := newSchemaDB(t, "tellomi_official_squat")
	// 20261107 also inserts user 777001. Skip it so a refused 20261105 is the
	// only file that could have created that user.
	applyTeamgramSchema(t, db, "tellomi_official_squat", false, "migrate-20261107-tellomi-ai-name.sql")
	if _, err := db.Exec(`INSERT INTO username (username, peer_type, peer_id, editable, active, order2, deleted) VALUES ('support', 2, 99, 1, 1, 0, 0)`); err != nil {
		t.Fatal(err)
	}
	err := mysqlFile(t, "tellomi_official_squat", "migrate-20261105-official-accounts.sql")
	if err == nil || !strings.Contains(err.Error(), "D-55: official username taken: support") {
		t.Fatalf("migration error = %v", err)
	}
	var peer int64
	if err := db.QueryRow(`SELECT peer_id FROM username WHERE username = 'support'`).Scan(&peer); err != nil {
		t.Fatal(err)
	}
	if peer != 99 {
		t.Fatalf("support peer = %d", peer)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = 777001`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("migration inserted users after refusing support")
	}
}

func TestAppendixCPlaceholdersDoNotSteal(t *testing.T) {
	db := newSchemaDB(t, "tellomi_appendix_c_squat")
	applyTeamgramSchema(t, db, "tellomi_appendix_c_squat", true, "migrate-20261109-appendix-c-placeholders.sql")
	if _, err := db.Exec(`INSERT INTO username (username, peer_type, peer_id, editable, active, order2, deleted) VALUES ('WeChat', 2, 42, 1, 1, 0, 0)`); err != nil {
		t.Fatal(err)
	}
	err := mysqlFile(t, "tellomi_appendix_c_squat", "migrate-20261109-appendix-c-placeholders.sql")
	if err == nil || !strings.Contains(err.Error(), "D-56: reserved username taken: WeChat") {
		t.Fatalf("migration error = %v", err)
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
}

func assertOfficialAccounts(t *testing.T, db *sql.DB) {
	t.Helper()
	want := []officialName{
		{"tellomi", 2, 777001, 1, 1},
		{"tellomi_official", 2, 777002, 1, 1},
		{"tellomiapp", 2, 777002, 0, 1},
		{"tellomi_team", 2, 777003, 1, 1},
		{"support", 2, 777004, 1, 1},
		{"tellomi_support", 2, 777004, 0, 1},
		{"kefu", 2, 777004, 0, 1},
		{"customer_support", 2, 777004, 0, 1},
		{"customer_service", 2, 777004, 0, 1},
		{"faxian", 2, 777005, 1, 1},
		{"discover", 2, 777005, 0, 1},
		{"admin", 2, 777000, 1, 1},
		{"tellomi_admin", 2, 777000, 0, 1},
		{"administrator", 2, 777000, 0, 1},
		{"system", 2, 777000, 0, 1},
		{"xitong", 2, 777000, 0, 1},
		{"noreply", 2, 777000, 0, 1},
		{"notifications", 2, 777000, 0, 1},
		{"news", 4, 1073800001, 1, 1},
		{"gonggao", 4, 1073800001, 0, 1},
		{"announcement", 4, 1073800001, 0, 1},
		{"notice", 4, 1073800001, 0, 1},
		{"updates", 4, 1073800002, 1, 1},
		{"security", 4, 1073800003, 1, 1},
		{"tellomi_security", 4, 1073800003, 0, 1},
		{"anquan", 4, 1073800003, 0, 1},
		{"safety", 4, 1073800003, 0, 1},
		{"trust", 4, 1073800003, 0, 1},
		{"dev", 4, 1073800004, 1, 1},
		{"developer", 4, 1073800004, 0, 1},
		{"api", 4, 1073800004, 0, 1},
		{"help", 0, 0, 0, 0},
		{"helpcenter", 0, 0, 0, 0},
		{"help_center", 0, 0, 0, 0},
		{"bangzhu", 0, 0, 0, 0},
		{"verify", 0, 0, 0, 0},
		{"tellomi_verify", 0, 0, 0, 0},
		{"verified", 0, 0, 0, 0},
		{"renzheng", 0, 0, 0, 0},
		{"report", 0, 0, 0, 0},
		{"jubao", 0, 0, 0, 0},
		{"abuse", 0, 0, 0, 0},
		{"feedback", 0, 0, 0, 0},
		{"fankui", 0, 0, 0, 0},
		{"bots", 0, 0, 0, 0},
		{"premium", 0, 0, 0, 0},
		{"vip", 0, 0, 0, 0},
		{"huiyuan", 0, 0, 0, 0},
		{"stickers", 0, 0, 0, 0},
	}
	occupied, unresolved := usernamepolicy.AppendixCPlaceholders()
	for _, name := range unresolved {
		for _, w := range want {
			if w.name == name {
				t.Fatalf("short name %s is assigned in appendix D", name)
			}
		}
	}
	for _, name := range occupied {
		want = append(want, officialName{name, 0, 0, 0, 0})
	}
	rows, err := db.Query(`SELECT username, peer_type, peer_id, editable, active, deleted FROM username ORDER BY username`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]officialName{}
	for rows.Next() {
		var row officialName
		var deleted int
		if err := rows.Scan(&row.name, &row.peerType, &row.peerID, &row.editable, &row.active, &deleted); err != nil {
			t.Fatal(err)
		}
		if deleted != 0 {
			t.Fatalf("%s deleted", row.name)
		}
		got[row.name] = row
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("username rows = %d, want %d", len(got), len(want))
	}
	for _, w := range want {
		g, ok := got[w.name]
		if !ok || g != w {
			t.Fatalf("username %s = %+v, want %+v", w.name, g, w)
		}
	}
	type userRow struct {
		id        int64
		userType  int
		first     string
		username  string
		phoneNull bool
		verified  int
		support   int
	}
	users := []userRow{
		{777000, 4, "Tellomi", "admin", false, 1, 0},
		{777001, 2, "Tellomi AI", "tellomi", true, 1, 0},
		{777002, 2, "Tellomi", "tellomi_official", true, 1, 0},
		{777003, 2, "Tellomi Team", "tellomi_team", true, 1, 0},
		{777004, 2, "Tellomi Support", "support", true, 1, 1},
		{777005, 2, "发现", "faxian", true, 1, 0},
	}
	for _, w := range users {
		var g userRow
		err := db.QueryRow(`SELECT id, user_type, first_name, username, phone IS NULL, verified, support FROM users WHERE id = ?`, w.id).
			Scan(&g.id, &g.userType, &g.first, &g.username, &g.phoneNull, &g.verified, &g.support)
		if err != nil {
			t.Fatalf("user %d: %v", w.id, err)
		}
		if g != w {
			t.Fatalf("user %d = %+v, want %+v", w.id, g, w)
		}
	}
	var phone string
	if err := db.QueryRow(`SELECT phone FROM users WHERE id = 777000`).Scan(&phone); err != nil {
		t.Fatal(err)
	}
	if phone != "42777" {
		t.Fatalf("service phone = %q", phone)
	}
	channels := []struct {
		id    int64
		title string
		name  string
	}{
		{1073800001, "官方公告", "news"},
		{1073800002, "更新日志", "updates"},
		{1073800003, "安全中心", "security"},
		{1073800004, "开发者", "dev"},
	}
	for _, w := range channels {
		var title, name string
		var creator int64
		err := db.QueryRow(`SELECT title, username, creator_user_id FROM channels WHERE id = ?`, w.id).Scan(&title, &name, &creator)
		if err != nil {
			t.Fatalf("channel %d: %v", w.id, err)
		}
		if title != w.title || name != w.name || creator != 777002 {
			t.Fatalf("channel %d = %s %s creator %d", w.id, title, name, creator)
		}
		var role, state int
		err = db.QueryRow(`SELECT role, state FROM channel_participants WHERE channel_id = ? AND user_id = 777002`, w.id).Scan(&role, &state)
		if err != nil {
			t.Fatalf("participant %d: %v", w.id, err)
		}
		if role != 1 || state != 0 {
			t.Fatalf("participant %d role %d state %d", w.id, role, state)
		}
	}
	var next int64
	if err := db.QueryRow(`SELECT next_id FROM channel_id_counter WHERE id = 1`).Scan(&next); err != nil {
		t.Fatal(err)
	}
	if next < 1073800005 {
		t.Fatalf("channel counter = %d", next)
	}
	var peerType int
	var peerID int64
	if err := db.QueryRow(`SELECT peer_type, peer_id FROM username WHERE username = 'WeChat'`).Scan(&peerType, &peerID); err != nil {
		t.Fatal(err)
	}
	if peerType != 0 || peerID != 0 {
		t.Fatalf("WeChat => type %d peer %d", peerType, peerID)
	}
	var tellomiN int
	if err := db.QueryRow(`SELECT COUNT(*) FROM username WHERE username = 'TELLOMI'`).Scan(&tellomiN); err != nil {
		t.Fatal(err)
	}
	if tellomiN != 1 {
		t.Fatalf("TELLOMI rows = %d", tellomiN)
	}
}

func newSchemaDB(t *testing.T, name string) *sql.DB {
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
	if _, err = exec.LookPath("mysql"); err != nil {
		t.Skip("mysql client not installed")
	}
	if _, err = db.Exec("DROP DATABASE IF EXISTS " + name); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP DATABASE " + name) })
	if _, err = db.Exec("USE " + name); err != nil {
		t.Fatal(err)
	}
	return db
}

func applyTeamgramSchema(t *testing.T, db *sql.DB, name string, official bool, skip ...string) {
	t.Helper()
	skipSet := map[string]bool{}
	for _, s := range skip {
		skipSet[s] = true
	}
	base, err := os.ReadFile("1_teamgram.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(base), "use teamgram;", "use "+name+";", 1)
	if err = mysqlText(t, name, text); err != nil {
		t.Fatal(err)
	}
	if err = mysqlFile(t, name, "0_migrate_helpers.sql"); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob("migrate-*.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	for _, f := range files {
		base := filepath.Base(f)
		if !official && base == "migrate-20261105-official-accounts.sql" {
			continue
		}
		if skipSet[base] {
			continue
		}
		if err = mysqlFile(t, name, f); err != nil {
			t.Fatal(err)
		}
	}
	if official {
		if err = mysqlFile(t, name, "z_init.sql"); err != nil {
			t.Fatal(err)
		}
	}
	_ = db
}

func mysqlFile(t *testing.T, dbName, path string) error {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return mysqlText(t, dbName, string(b))
}

func mysqlText(t *testing.T, dbName, text string) error {
	t.Helper()
	cmd := exec.Command("mysql", "--protocol=TCP", "-h", "127.0.0.1", "-u", "teamgram", "--default-character-set=utf8mb4", dbName)
	cmd.Env = append(os.Environ(), "MYSQL_PWD=teamgram")
	cmd.Stdin = strings.NewReader(text)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mysql %s: %w\n%s", dbName, err, out)
	}
	return nil
}
