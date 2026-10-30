package sqlmigrate

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
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
	for _, name := range []string{"teamgram_add_column", "teamgram_drop_column", "teamgram_add_index", "teamgram_drop_index", "teamgram_change_column"} {
		if !strings.Contains(helpers, "PROCEDURE "+name) {
			t.Errorf("helper %s missing", name)
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
