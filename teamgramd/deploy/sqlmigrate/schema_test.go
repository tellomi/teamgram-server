package sqlmigrate

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/usernamepolicy"
)

func sqlDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "sql")
	if _, err := os.Stat(filepath.Join(dir, "1_teamgram.sql")); err != nil {
		t.Fatalf("schema not found at %s: %v", dir, err)
	}
	return dir
}

// TestMigrationsMatchCodeSchema applies 1_teamgram.sql and every migrate-*.sql
// the same way a fresh install does, then checks columns the chat service
// selects. Production missed migrate-20260610.sql, so chat_participants.rank2
// was absent and SelectList failed with 1054.
func TestMigrationsMatchCodeSchema(t *testing.T) {
	dir := sqlDir(t)
	cols := parseCreateTables(t, filepath.Join(dir, "1_teamgram.sql"))
	files, err := filepath.Glob(filepath.Join(dir, "migrate-*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	for _, f := range files {
		applyMigration(cols, mustRead(t, f))
	}

	for _, col := range []string{
		"groupcall_default_join_as_peer_type",
		"groupcall_default_join_as_peer_id",
		"is_bot",
		"rank2",
	} {
		if !cols["chat_participants"][col] {
			t.Errorf("chat_participants.%s missing after migrations", col)
		}
	}
	for _, col := range []string{"chat_id", "requested", "approved_by"} {
		if !cols["chat_invite_participants"][col] {
			t.Errorf("chat_invite_participants.%s missing after migrations", col)
		}
	}
	if cols["chat_participants"]["rank2"] && !strings.Contains(mustRead(t, filepath.Join(dir, "migrate-20260610.sql")), "rank2") {
		t.Fatal("rank2 is not produced by migrate-20260610.sql")
	}
	if !cols["users"]["email"] {
		t.Fatal("users.email missing after migrations")
	}
	for _, col := range []string{"media_document_id", "media_photo_id"} {
		if !cols["messages"][col] {
			t.Errorf("messages.%s missing after migrations", col)
		}
	}
	if !cols["documents"]["sha256"] || !cols["documents"]["blob_id"] {
		t.Fatal("documents sha256/blob_id missing after migrations")
	}
	for _, col := range []string{"sha256", "file_size", "file_path", "storage_class", "ref_count", "origin_ref_id"} {
		if !cols["media_blobs"][col] {
			t.Errorf("media_blobs.%s missing after migrations", col)
		}
	}
	if !cols["media_blob_refs"]["ref_id"] || !cols["media_photo_paths"]["file_path"] {
		t.Fatal("media dedup tables missing after migrations")
	}
	for _, col := range []string{"storage_class", "file_path"} {
		if !cols["media_blob_gc"][col] {
			t.Errorf("media_blob_gc.%s missing after migrations", col)
		}
	}
	for _, col := range []string{"viewer_user_id", "owner_kind", "owner_id", "story_id", "deleted_at"} {
		if !cols["story_deletions"][col] {
			t.Errorf("story_deletions.%s missing after migrations", col)
		}
	}
	for _, col := range []string{"encrypted_requests_disabled", "call_requests_disabled", "unconfirmed", "activity_trusted"} {
		if !cols["auth_users"][col] {
			t.Errorf("auth_users.%s missing after migrations", col)
		}
	}
	settingsMig := mustRead(t, filepath.Join(dir, "migrate-20261103-authorization-settings.sql"))
	for _, col := range []string{"encrypted_requests_disabled", "call_requests_disabled", "unconfirmed"} {
		if !strings.Contains(settingsMig, "teamgram_add_column('auth_users', '"+col+"'") {
			t.Errorf("authorization settings migration does not add auth_users.%s", col)
		}
	}
	emailMig := mustRead(t, filepath.Join(dir, "migrate-20261030-email.sql"))
	if !strings.Contains(emailMig, "teamgram_add_column('users', 'email'") || !strings.Contains(emailMig, "MODIFY `phone`") {
		t.Fatal("email migration does not add users.email or relax users.phone")
	}
}

func TestMigrateFilesAreIdempotent(t *testing.T) {
	dir := sqlDir(t)
	files, err := filepath.Glob(filepath.Join(dir, "migrate-*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no migrations")
	}
	for _, f := range files {
		for i, line := range strings.Split(mustRead(t, f), "\n") {
			s := strings.TrimSpace(line)
			if s == "" || strings.HasPrefix(s, "--") || strings.HasPrefix(s, "/*") || strings.HasPrefix(s, "*") || strings.HasPrefix(s, "/*!") {
				continue
			}
			upper := strings.ToUpper(s)
			switch {
			case strings.HasPrefix(upper, "ALTER "),
				strings.HasPrefix(upper, "DROP "),
				strings.HasPrefix(upper, "RENAME "),
				strings.HasPrefix(upper, "TRUNCATE "),
				strings.HasPrefix(upper, "UPDATE "),
				strings.HasPrefix(upper, "DELETE "):
				t.Errorf("%s:%d is not idempotent: %s", filepath.Base(f), i+1, s)
			case strings.HasPrefix(upper, "CREATE TABLE ") && !strings.Contains(upper, "IF NOT EXISTS"):
				t.Errorf("%s:%d create table is not idempotent: %s", filepath.Base(f), i+1, s)
			case strings.HasPrefix(upper, "INSERT ") && !strings.HasPrefix(upper, "INSERT IGNORE"):
				t.Errorf("%s:%d insert is not idempotent: %s", filepath.Base(f), i+1, s)
			}
		}
	}
	z := mustRead(t, filepath.Join(dir, "z_init.sql"))
	if strings.Contains(z, "INSERT INTO ") && !strings.Contains(z, "INSERT IGNORE INTO ") {
		t.Fatal("z_init.sql insert is not idempotent")
	}
	helpers := mustRead(t, filepath.Join(dir, "0_migrate_helpers.sql"))
	for _, name := range []string{"teamgram_add_column", "teamgram_drop_column", "teamgram_add_index", "teamgram_drop_index", "teamgram_change_column", "teamgram_fail_if_set"} {
		if !strings.Contains(helpers, "PROCEDURE "+name) {
			t.Errorf("helper %s missing", name)
		}
	}
}

func TestDialogHistoryIndexIsOnline(t *testing.T) {
	raw := mustRead(t, filepath.Join(sqlDir(t), "migrate-20261126-dialog-history-index.sql"))
	if !strings.Contains(raw, "CALL teamgram_add_index('messages', 'messages_dialog_history'") {
		t.Fatal("dialog history index is not created through the idempotent helper")
	}
	stmt := "ALTER TABLE `messages` ADD INDEX `messages_dialog_history` (`user_id`, `dialog_id1`, `dialog_id2`, `deleted`, `user_message_box_id`), ALGORITHM=INPLACE, LOCK=NONE"
	if !strings.Contains(raw, stmt) {
		t.Fatal("dialog history index is not an online InnoDB add")
	}
	for _, line := range strings.Split(raw, "\n") {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(s), "ALTER ") {
			t.Fatalf("raw ALTER is not idempotent: %s", s)
		}
	}
	query := mustRead(t, filepath.Join("..", "..", "..", "app", "service", "biz", "message", "internal", "dal", "dao", "mysql_dao", "dialog_history_query.go"))
	for _, piece := range []string{
		"user_id = ?",
		"dialog_id1 = ?",
		"dialog_id2 = ?",
		"deleted = 0",
		"user_message_box_id < ?",
		"user_message_box_id > ?",
		"user_message_box_id >= ?",
		"order by user_message_box_id desc",
		"order by user_message_box_id asc",
	} {
		if !strings.Contains(query, piece) {
			t.Fatalf("SelectDialogHistory no longer matches the index: missing %s", piece)
		}
	}
}

func TestMessageRandomIDIndexIsOnline(t *testing.T) {
	raw := mustRead(t, filepath.Join(sqlDir(t), "migrate-20261111-message-random-id.sql"))
	if !strings.Contains(raw, "CALL teamgram_add_index('messages', 'sender_random_id'") {
		t.Fatal("random_id index is not created through the idempotent helper")
	}
	stmt := "ALTER TABLE `messages` ADD INDEX `sender_random_id` (`sender_user_id`, `random_id`), ALGORITHM=INPLACE, LOCK=NONE"
	if !strings.Contains(raw, stmt) {
		t.Fatal("random_id index is not an online InnoDB add")
	}
	for _, line := range strings.Split(raw, "\n") {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(s), "ALTER ") {
			t.Fatalf("raw ALTER is not idempotent: %s", s)
		}
	}
}

func TestOfficialAccountMigration(t *testing.T) {
	dir := sqlDir(t)
	raw := mustRead(t, filepath.Join(dir, "migrate-20261105-official-accounts.sql"))
	for _, name := range []string{
		"tellomi", "tellomi_official", "tellomiapp", "tellomi_team",
		"support", "tellomi_support", "kefu", "customer_support", "customer_service",
		"faxian", "discover",
		"admin", "tellomi_admin", "administrator", "system", "xitong", "noreply", "notifications",
		"news", "gonggao", "announcement", "notice", "updates",
		"security", "tellomi_security", "anquan", "safety", "trust",
		"dev", "developer", "api",
		"help", "helpcenter", "help_center", "bangzhu",
		"verify", "tellomi_verify", "verified", "renzheng",
		"report", "jubao", "abuse", "feedback", "fankui",
		"bots", "premium", "vip", "huiyuan", "stickers",
		"777001", "1073800001", "D-55", "peer_type = 0",
	} {
		if !strings.Contains(raw, "'"+name+"'") && !strings.Contains(raw, name) {
			t.Fatalf("official account migration missing %s", name)
		}
	}
	for _, blocked := range []string{"wechat", "google", "whatsapp", "openai", "qq"} {
		if strings.Contains(strings.ToLower(raw), blocked) {
			t.Fatalf("appendix C name %s was written in the official-account migration", blocked)
		}
	}
	z := mustRead(t, filepath.Join(dir, "z_init.sql"))
	if !strings.Contains(z, "'Tellomi', '', 'admin', '42777'") {
		t.Fatal("z_init.sql service user username is not admin")
	}
	files, err := filepath.Glob(filepath.Join(dir, "migrate-*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	var seenTellomi, seenOfficial bool
	for _, f := range files {
		base := filepath.Base(f)
		if base == "migrate-20261104-tellomi-username.sql" {
			seenTellomi = true
		}
		if base == "migrate-20261105-official-accounts.sql" {
			if !seenTellomi {
				t.Fatal("official accounts migration sorts before the tellomi username migration")
			}
			seenOfficial = true
		}
	}
	if !seenOfficial {
		t.Fatal("official accounts migration missing")
	}
}

func TestOfficialUsernameRunbook(t *testing.T) {
	root := filepath.Join(sqlDir(t), "..", "..", "..")
	doc := mustRead(t, filepath.Join(root, "docs", "official-usernames.md"))
	for _, s := range []string{
		"先备份",
		"FLUSHALL",
		"migrate-20261104-tellomi-username.sql",
		"migrate-20261105-official-accounts.sql",
		"migrate-20261107-tellomi-ai-name.sql",
		"migrate-20261109-appendix-c-placeholders.sql",
		"D-27",
		"777001",
		"peer_type",
		"qq",
	} {
		if !strings.Contains(doc, s) {
			t.Fatalf("official username runbook missing %s", s)
		}
	}
	patches := mustRead(t, filepath.Join(root, "UPSTREAM_PATCHES.md"))
	for _, s := range []string{"z_init.sql", "D-27", "teamgram_fail_if_set", "docs/official-usernames.md"} {
		if !strings.Contains(patches, s) {
			t.Fatalf("UPSTREAM_PATCHES.md missing %s", s)
		}
	}
}

func TestAppendixCPlaceholderMigration(t *testing.T) {
	occupied, unresolved := usernamepolicy.AppendixCPlaceholders()
	raw := mustRead(t, filepath.Join(sqlDir(t), "migrate-20261109-appendix-c-placeholders.sql"))
	re := regexp.MustCompile(`\('([a-z0-9_]+)', 0, 0, 0, 0, 0, 0\)`)
	found := map[string]int{}
	for _, m := range re.FindAllStringSubmatch(raw, -1) {
		found[m[1]]++
	}
	if len(found) != len(occupied) {
		t.Fatalf("placeholder rows %d, list %d", len(found), len(occupied))
	}
	for _, name := range occupied {
		if found[name] != 1 {
			t.Fatalf("%s count %d", name, found[name])
		}
	}
	for _, name := range unresolved {
		if strings.Contains(raw, "'"+name+"'") {
			t.Fatalf("short name %s was written", name)
		}
	}
	for _, s := range []string{"D-56", "peer_type 0", "teamgram_fail_if_set", "INSERT IGNORE INTO username", "utf8mb4_unicode_ci"} {
		if !strings.Contains(raw, s) {
			t.Fatalf("appendix C migration missing %s", s)
		}
	}
}

func TestTellomiUsernameMigration(t *testing.T) {
	raw := mustRead(t, filepath.Join(sqlDir(t), "migrate-20261104-tellomi-username.sql"))
	for _, s := range []string{
		"tellomi",
		"777000",
		"peer_type = 2",
		"peer_id = 777000",
		"utf8mb4_unicode_ci",
		"D-27",
		"INSERT IGNORE INTO username",
	} {
		if !strings.Contains(raw, s) {
			t.Fatalf("tellomi username migration missing %s", s)
		}
	}
	for _, line := range strings.Split(raw, "\n") {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(s), "UPDATE ") || strings.HasPrefix(strings.ToUpper(s), "DELETE ") {
			t.Fatalf("raw UPDATE/DELETE is not idempotent under the migration check: %s", s)
		}
	}
}

func TestDiscoverPushMigration(t *testing.T) {
	raw := mustRead(t, filepath.Join(sqlDir(t), "migrate-20261106-discover-push.sql"))
	if !strings.Contains(raw, "CREATE TABLE IF NOT EXISTS discover_push_event") || !strings.Contains(raw, "event_id") {
		t.Fatal("discover push migration is missing the event table")
	}
}

func TestTellomiAIReplyMigration(t *testing.T) {
	raw := mustRead(t, filepath.Join(sqlDir(t), "migrate-20261106-tellomi-ai.sql"))
	if !strings.Contains(raw, "CREATE TABLE IF NOT EXISTS tellomi_ai_reply") || !strings.Contains(raw, "prompt") || !strings.Contains(raw, "reply") {
		t.Fatal("tellomi ai reply migration is missing the audit table")
	}
}

func TestAnnounceMigration(t *testing.T) {
	raw := mustRead(t, filepath.Join(sqlDir(t), "migrate-20261108-announce.sql"))
	if !strings.Contains(raw, "CREATE TABLE IF NOT EXISTS tellomi_announcement") || !strings.Contains(raw, "CREATE TABLE IF NOT EXISTS tellomi_announcement_channel") {
		t.Fatal("announce migration is missing a claim table")
	}
	if strings.Contains(strings.ToUpper(raw), "FLUSH") {
		t.Fatal("announce migration must not flush redis")
	}
}

func TestServiceUserRenameMigration(t *testing.T) {
	raw := mustRead(t, filepath.Join(sqlDir(t), "migrate-20261101-service-user.sql"))
	for _, s := range []string{"777000", "Tellomi", "Telegram", "Teamgram", "tellomi"} {
		if !strings.Contains(raw, s) {
			t.Fatalf("service-user migration missing %s", s)
		}
	}
	for _, line := range strings.Split(raw, "\n") {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(s), "UPDATE ") {
			t.Fatalf("raw UPDATE is not idempotent under the migration check: %s", s)
		}
	}
}

func TestAdvertSQLIsNotAppliedOnStartup(t *testing.T) {
	dir := sqlDir(t)
	root := filepath.Join(dir, "..")
	files, err := filepath.Glob(filepath.Join(dir, "migrate-*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.Contains(mustRead(t, f), "story_advert_dirty") || strings.Contains(mustRead(t, f), "story_live_owner") {
			t.Fatalf("%s is on the automatic migrate path", filepath.Base(f))
		}
	}
	gated := filepath.Join(root, "gated", "20261120-story-advert.sql")
	if _, err = os.Stat(gated); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"migrate.sh", filepath.Join("..", "docker", "entrypoint.sh")} {
		text := mustRead(t, filepath.Join(root, name))
		if strings.Contains(text, "apply-gated-sql") || strings.Contains(text, "gated/") {
			t.Fatalf("%s invokes the gated advert SQL", name)
		}
	}
	schema := mustRead(t, filepath.Join(root, "..", "..", "app", "service", "biz", "story", "schema.go"))
	if strings.Contains(schema, "story_advert_dirty") || strings.Contains(schema, "story_live_owner") {
		t.Fatal("story schema startup creates advert tables")
	}
	script := filepath.Join(root, "apply-gated-sql.sh")
	cmd := exec.Command("bash", script)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("gate ran without a backup confirmation: %s", out)
	}
	if !strings.Contains(string(out), "TEAMGRAM_SQL_BACKUP_CONFIRMED") {
		t.Fatalf("gate output %q err %v", out, err)
	}
}

func TestMediaJSONColumnsMigration(t *testing.T) {
	dir := sqlDir(t)
	raw := mustRead(t, filepath.Join(dir, "migrate-20261125-media-json-columns.sql"))
	for _, s := range []string{
		"$.media.document.id",
		"$.media.photo_FLAGPHOTO.id",
		"GENERATED ALWAYS AS",
		"STORED",
		"JSON_EXTRACT",
		"JSON_UNQUOTE",
		"REGEXP_REPLACE",
		"ALGORITHM=COPY, LOCK=SHARED",
		"write-freeze",
		"PREPARE teamgram_ps FROM @teamgram_stmt",
		"ADD COLUMN `media_document_id`",
		"ADD COLUMN `media_photo_id`",
		"ADD INDEX `media_document_id`",
		"ADD INDEX `media_photo_id`",
		"DROP INDEX `media_document_id`",
		"DROP INDEX `media_photo_id`",
	} {
		if !strings.Contains(raw, s) {
			t.Errorf("media json migration missing %s", s)
		}
	}
	if strings.Count(raw, "ADD COLUMN `media_document_id`") != 1 || strings.Count(raw, "ADD COLUMN `media_photo_id`") != 1 {
		t.Fatal("media id columns are not added exactly once")
	}
	alter := raw[strings.Index(raw, "ADD COLUMN `media_document_id`"):]
	if !strings.Contains(alter, "ADD COLUMN `media_photo_id`") || !strings.Contains(alter, "ADD INDEX `media_photo_id`") {
		t.Fatal("both columns and both indexes are not in one ALTER")
	}
	if strings.Contains(raw, "TRIM(LEADING '' \t") || strings.Contains(raw, "TRIM(LEADING '' \\t") {
		t.Fatal("TRIM still removes a space+tab prefix instead of each whitespace character")
	}
	for _, line := range strings.Split(raw, "\n") {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(s, "--") {
			continue
		}
		if strings.Contains(s, "JSON_VALUE") {
			t.Fatalf("executable JSON_VALUE remains: %s", s)
		}
	}
	old := mustRead(t, filepath.Join(dir, "migrate-20261103-media-retention.sql"))
	if strings.Contains(old, "JSON_VALUE") {
		t.Fatal("media-retention migration still creates a JSON_VALUE index")
	}
	root := filepath.Join(dir, "..", "..", "..")
	for _, rel := range []string{
		"app/service/media/retention/store.go",
		"app/service/mediapurge/schema.go",
		"app/bff/account/deletion/wipe/sql.go",
		"pkg/mediaaccess/store.go",
		filepath.Join("e2e", "client", "media_retention_test.go"),
	} {
		text := mustRead(t, filepath.Join(root, rel))
		if strings.Contains(text, "JSON_VALUE") {
			t.Errorf("%s still queries JSON_VALUE", rel)
		}
	}
}

func TestMigrateScriptParsesDSN(t *testing.T) {
	script := filepath.Join("..", "migrate.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatal(err)
	}
	// Sourced, not executed: the guard at the bottom only migrates when the file is the process.
	out, err := runBash(t, script)
	if err != nil {
		t.Fatal(err)
	}
	if out != "teamgram secret db.internal 3307 teamgram" {
		t.Fatalf("parsed dsn = %q", out)
	}
}

func TestMigrateScriptSSLArgs(t *testing.T) {
	script := filepath.Join("..", "migrate.sh")
	dir := t.TempDir()
	mock := filepath.Join(dir, "mysql")
	if err := os.WriteFile(mock, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(ca string) string {
		t.Helper()
		cmd := "source " + script + "\nMYSQL_PASSWORD=secret mysql_cli -e 'SELECT 1'\n"
		c := exec.Command("bash", "-c", cmd)
		c.Env = []string{"PATH=" + dir + ":" + os.Getenv("PATH"), "TEAMGRAM_MYSQL_TLS_CA=" + ca}
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("ca %q: %v\n%s", ca, err, out)
		}
		return string(out)
	}
	plain := run("")
	if strings.Contains(plain, "ssl-mode") || strings.Contains(plain, "ssl-ca") {
		t.Fatalf("ssl flags without a CA:\n%s", plain)
	}
	with := run("/opt/tellomi/ca.pem")
	if !strings.Contains(with, "--ssl-mode=VERIFY_CA") || !strings.Contains(with, "--ssl-ca=/opt/tellomi/ca.pem") {
		t.Fatalf("ssl flags:\n%s", with)
	}
}

func parseCreateTables(t *testing.T, path string) map[string]map[string]bool {
	t.Helper()
	text := mustRead(t, path)
	tables := map[string]map[string]bool{}
	re := regexp.MustCompile("(?s)CREATE TABLE `(\\w+)` \\((.*?)\\) ENGINE")
	colRe := regexp.MustCompile("^`(\\w+)`")
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		cols := map[string]bool{}
		for _, line := range strings.Split(m[2], "\n") {
			line = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), ","))
			if c := colRe.FindStringSubmatch(line); c != nil {
				cols[c[1]] = true
			}
		}
		tables[m[1]] = cols
	}
	if len(tables) == 0 {
		t.Fatal("no tables in base schema")
	}
	return tables
}

func applyMigration(cols map[string]map[string]bool, text string) {
	addRe := regexp.MustCompile(`teamgram_add_column\('(\w+)', '(\w+)'`)
	dropRe := regexp.MustCompile(`teamgram_drop_column\('(\w+)', '(\w+)'`)
	createRe := regexp.MustCompile("(?s)CREATE TABLE IF NOT EXISTS `(\\w+)` \\((.*?)\\) ENGINE")
	colRe := regexp.MustCompile("^`?(\\w+)`?")
	for _, m := range addRe.FindAllStringSubmatch(text, -1) {
		set := cols[m[1]]
		if set == nil {
			set = map[string]bool{}
			cols[m[1]] = set
		}
		set[m[2]] = true
	}
	for _, m := range dropRe.FindAllStringSubmatch(text, -1) {
		if set := cols[m[1]]; set != nil {
			delete(set, m[2])
		}
	}
	for _, m := range createRe.FindAllStringSubmatch(text, -1) {
		set := cols[m[1]]
		if set == nil {
			set = map[string]bool{}
			cols[m[1]] = set
		}
		for _, line := range strings.Split(m[2], "\n") {
			line = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), ","))
			upper := strings.ToUpper(line)
			if strings.HasPrefix(upper, "PRIMARY") || strings.HasPrefix(upper, "UNIQUE") || strings.HasPrefix(upper, "KEY") || strings.HasPrefix(upper, "CONSTRAINT") {
				continue
			}
			if c := colRe.FindStringSubmatch(line); c != nil {
				set[c[1]] = true
			}
		}
	}
	for _, col := range []string{"media_document_id", "media_photo_id"} {
		if strings.Contains(text, "ADD COLUMN `"+col+"`") {
			set := cols["messages"]
			if set == nil {
				set = map[string]bool{}
				cols["messages"] = set
			}
			set[col] = true
		}
	}
	// date_actived -> date_active
	if strings.Contains(text, "CHANGE `date_actived` `date_active`") {
		if set := cols["auth_users"]; set != nil && set["date_actived"] {
			delete(set, "date_actived")
			set["date_active"] = true
		}
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func runBash(t *testing.T, script string) (string, error) {
	t.Helper()
	cmd := "source " + script + "\n" +
		"parse_mysql_dsn 'teamgram:secret@tcp(db.internal:3307)/teamgram?charset=utf8mb4'\n" +
		"printf '%s %s %s %s %s' \"$MYSQL_USER\" \"$MYSQL_PASSWORD\" \"$MYSQL_HOST\" \"$MYSQL_PORT\" \"$MYSQL_DATABASE\"\n"
	return bash(cmd)
}

func bash(script string) (string, error) {
	cmd := exec.Command("bash", "-c", script)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
