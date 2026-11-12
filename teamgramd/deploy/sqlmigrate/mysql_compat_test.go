package sqlmigrate

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// TestMySQLCompat applies every startup migration to MySQL 8.0.13 and 8.0.46
// and checks the generated media-id columns against JSON_VALUE semantics.
// TEAMGRAM_MYSQL_COMPAT=1 is set by the Go test workflow. Without it, go test
// ./... does not pull the server images.
func TestMySQLCompat(t *testing.T) {
	if os.Getenv("TEAMGRAM_MYSQL_COMPAT") == "" {
		t.Skip("set TEAMGRAM_MYSQL_COMPAT=1 to apply migrations on MySQL 8.0.13 and 8.0.46")
	}
	bin := dockerBin(t)
	dir := sqlDir(t)

	type server struct {
		image    string
		name     string
		jsonFunc bool
	}
	servers := []server{
		{image: "mysql:8.0.13", name: "teamgram-compat-8013", jsonFunc: false},
		{image: "mysql:8.0.46", name: "teamgram-compat-8046", jsonFunc: true},
	}
	for _, srv := range servers {
		t.Run(srv.image, func(t *testing.T) {
			port := startMySQL(t, bin, srv.image, srv.name)
			applyStartup(t, bin, srv.name, dir)
			applyStartupAgain(t, bin, srv.name, dir)
			db := openMySQL(t, port, "teamgram")
			checkGeneratedMediaIDs(t, db, srv.jsonFunc)
			checkDialogHistoryIndex(t, db)
			if srv.jsonFunc {
				checkLegacyFunctionalIndex(t, bin, srv.name, port, dir)
			}
			checkSubjectsByUsers(t, db, port)
		})
	}
}

func checkGeneratedMediaIDs(t *testing.T, db *sql.DB, jsonFunc bool) {
	t.Helper()
	const userID = 900001
	if _, err := db.Exec(`DELETE FROM messages WHERE user_id=?`, userID); err != nil {
		t.Fatal(err)
	}
	for i, fx := range mediaFixtures {
		var data any
		if !fx.null {
			data = fx.raw
		}
		if _, err := db.Exec(`INSERT INTO messages
(user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, message_data)
VALUES (?,?,1,1,?,1,2,1,?)`, userID, i+1, i+1, data); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	rows, err := db.Query(`SELECT user_message_box_id, media_document_id, media_photo_id FROM messages WHERE user_id=? ORDER BY user_message_box_id`, userID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var box int
		var doc, photo sql.NullInt64
		if err = rows.Scan(&box, &doc, &photo); err != nil {
			t.Fatal(err)
		}
		fx := mediaFixtures[n]
		if !nullEq(doc, fx.doc) || !nullEq(photo, fx.photo) {
			t.Errorf("box %d doc=%v photo=%v, want doc=%v photo=%v json=%s", box, nullText(doc), nullText(photo), ptrText(fx.doc), ptrText(fx.photo), fx.raw)
		}
		n++
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if n != len(mediaFixtures) {
		t.Fatalf("rows %d, fixtures %d", n, len(mediaFixtures))
	}
	if got := explainKey(t, db, `EXPLAIN SELECT user_message_box_id FROM messages WHERE media_document_id = 123`); got != "media_document_id" {
		t.Fatalf("EXPLAIN key %q, want media_document_id", got)
	}
	if !jsonFunc {
		return
	}
	rows, err = db.Query(`SELECT
media_document_id,
JSON_VALUE(message_data, '$.media.document.id' RETURNING SIGNED NULL ON EMPTY NULL ON ERROR),
media_photo_id,
JSON_VALUE(message_data, '$.media.photo_FLAGPHOTO.id' RETURNING SIGNED NULL ON EMPTY NULL ON ERROR)
FROM messages WHERE user_id=? ORDER BY user_message_box_id`, userID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var a, b, c, d sql.NullInt64
		if err = rows.Scan(&a, &b, &c, &d); err != nil {
			t.Fatal(err)
		}
		if a != b || c != d {
			t.Errorf("column (%s,%s) != JSON_VALUE (%s,%s)", nullText(a), nullText(c), nullText(b), nullText(d))
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	var byCol, byFn int
	if err = db.QueryRow(`SELECT COUNT(*) FROM messages WHERE user_id=? AND media_document_id = 123`, userID).Scan(&byCol); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM messages WHERE user_id=? AND JSON_VALUE(message_data, '$.media.document.id' RETURNING SIGNED NULL ON EMPTY NULL ON ERROR) = 123`, userID).Scan(&byFn); err != nil {
		t.Fatal(err)
	}
	// 123, "123", and " 123" are three fixtures that all convert to 123.
	if byCol != byFn || byCol != 3 {
		t.Fatalf("document 123 rows column=%d json=%d", byCol, byFn)
	}
}

// checkDialogHistoryIndex loads a hot dialog and a cold one, checks that
// SelectDialogHistory's backward and forward pages match an in-memory filter,
// and that EXPLAIN uses messages_dialog_history without a filesort.
func checkDialogHistoryIndex(t *testing.T, db *sql.DB) {
	t.Helper()
	var cols int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'messages' AND INDEX_NAME = 'messages_dialog_history'`).Scan(&cols); err != nil {
		t.Fatal(err)
	}
	if cols != 5 {
		t.Fatalf("messages_dialog_history has %d columns, want 5", cols)
	}
	const (
		user  = int64(910001)
		hot1  = int64(11)
		hot2  = int64(22)
		cold1 = int64(33)
		cold2 = int64(44)
	)
	if _, err := db.Exec(`DELETE FROM messages WHERE user_id = ?`, user); err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO messages
		(user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, deleted)
		VALUES (?,?,?,?,?,1,2,1,?)`
	var live []int
	for i := 1; i <= 400; i++ {
		deleted := 0
		if i%5 == 0 {
			deleted = 1
		} else {
			live = append(live, i)
		}
		if _, err := db.Exec(insert, user, i, hot1, hot2, i, deleted); err != nil {
			t.Fatalf("hot %d: %v", i, err)
		}
	}
	for i := 1; i <= 40; i++ {
		// user_message_box_id is unique per user, so the cold dialog cannot
		// reuse the hot dialog's box ids.
		if _, err := db.Exec(insert, user, 10000+i, cold1, cold2, i, 0); err != nil {
			t.Fatalf("cold %d: %v", i, err)
		}
	}
	analyzed, err := db.Query(`ANALYZE TABLE messages`)
	if err != nil {
		t.Fatal(err)
	}
	analyzed.Close()
	// Newest live ids first. live is ascending, so reverse it.
	wantDesc := make([]int, len(live))
	for i := range live {
		wantDesc[i] = live[len(live)-1-i]
	}
	page1 := historyPage(t, db, user, hot1, hot2, 0, true, 20)
	if !sameInts(page1, wantDesc[:20]) {
		t.Fatalf("backward page 1 %v, want %v", page1, wantDesc[:20])
	}
	cursor := page1[len(page1)-1]
	page2 := historyPage(t, db, user, hot1, hot2, cursor, true, 20)
	if !sameInts(page2, wantDesc[20:40]) {
		t.Fatalf("backward page 2 %v, want %v", page2, wantDesc[20:40])
	}
	for _, id := range page2 {
		if id >= cursor {
			t.Fatalf("page 2 id %d is not below cursor %d", id, cursor)
		}
	}
	forward := historyPage(t, db, user, hot1, hot2, cursor, false, 20)
	// Forward is user_message_box_id >= cursor, oldest first, deleted skipped.
	var wantFwd []int
	for _, id := range live {
		if id >= cursor {
			wantFwd = append(wantFwd, id)
			if len(wantFwd) == 20 {
				break
			}
		}
	}
	if !sameInts(forward, wantFwd) {
		t.Fatalf("forward page %v, want %v", forward, wantFwd)
	}
	cold := historyPage(t, db, user, cold1, cold2, 0, true, 50)
	if len(cold) != 40 || cold[0] != 10040 || cold[len(cold)-1] != 10001 {
		t.Fatalf("cold dialog page %v", cold)
	}

	key, extra := explainSelect(t, db, `EXPLAIN SELECT user_message_box_id FROM messages
		WHERE user_id = ? AND dialog_id1 = ? AND dialog_id2 = ? AND deleted = 0 AND user_message_box_id < ?
		ORDER BY user_message_box_id DESC LIMIT 20`, user, hot1, hot2, cursor)
	t.Logf("EXPLAIN key=%s extra=%s", key, extra)
	if key != "messages_dialog_history" {
		t.Fatalf("EXPLAIN key %q, want messages_dialog_history (extra %s)", key, extra)
	}
	if strings.Contains(strings.ToLower(extra), "filesort") {
		t.Fatalf("EXPLAIN still sorts: %s", extra)
	}
}

func historyPage(t *testing.T, db *sql.DB, user, d1, d2 int64, cursor int, desc bool, limit int) []int {
	t.Helper()
	q := `SELECT user_message_box_id FROM messages
		WHERE user_id = ? AND dialog_id1 = ? AND dialog_id2 = ? AND deleted = 0`
	args := []any{user, d1, d2}
	if cursor > 0 && desc {
		q += ` AND user_message_box_id < ?`
		args = append(args, cursor)
	}
	if cursor > 0 && !desc {
		q += ` AND user_message_box_id >= ?`
		args = append(args, cursor)
	}
	if desc {
		q += ` ORDER BY user_message_box_id DESC LIMIT ?`
	} else {
		q += ` ORDER BY user_message_box_id ASC LIMIT ?`
	}
	args = append(args, limit)
	rows, err := db.Query(q, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var id int
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out = append(out, id)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func sameInts(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func explainSelect(t *testing.T, db *sql.DB, q string, args ...any) (key, extra string) {
	t.Helper()
	rows, err := db.Query(q, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatal("EXPLAIN returned no row")
	}
	vals := make([]sql.NullString, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err = rows.Scan(ptrs...); err != nil {
		t.Fatal(err)
	}
	for i, name := range cols {
		switch strings.ToLower(name) {
		case "key":
			key = vals[i].String
		case "extra":
			extra = vals[i].String
		}
	}
	return key, extra
}

func checkLegacyFunctionalIndex(t *testing.T, bin, container, port, dir string) {
	t.Helper()
	execSQL(t, bin, container, "mysql", `CREATE DATABASE IF NOT EXISTS legacy`)
	execSQL(t, bin, container, "legacy", `
CREATE TABLE messages (
  id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id bigint NOT NULL,
  user_message_box_id int NOT NULL,
  message_data json DEFAULT NULL,
  deleted tinyint NOT NULL DEFAULT 0
)`)
	execSQL(t, bin, container, "legacy", `
ALTER TABLE messages ADD INDEX media_document_id ((JSON_VALUE(message_data, '$.media.document.id' RETURNING SIGNED NULL ON EMPTY NULL ON ERROR)));
ALTER TABLE messages ADD INDEX media_photo_id ((JSON_VALUE(message_data, '$.media.photo_FLAGPHOTO.id' RETURNING SIGNED NULL ON EMPTY NULL ON ERROR)))`)
	db := openMySQL(t, port, "legacy")
	if _, err := db.Exec(`INSERT INTO messages (user_id, user_message_box_id, message_data) VALUES
(1,1,'{"media":{"document":{"id":7},"photo_FLAGPHOTO":{"id":"abc"}}}'),
(1,2,'{"media":{"document":{"id":"nope"}}}'),
(1,3,'{}')`); err != nil {
		t.Fatal(err)
	}
	mysqlFile(t, bin, container, "legacy", filepath.Join(dir, "0_migrate_helpers.sql"))
	mig := filepath.Join(dir, "migrate-20261125-media-json-columns.sql")
	mysqlFile(t, bin, container, "legacy", mig)
	mysqlFile(t, bin, container, "legacy", mig)
	var create string
	if err := db.QueryRow(`SHOW CREATE TABLE messages`).Scan(new(string), &create); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(create, "JSON_VALUE") {
		t.Fatalf("functional index still present:\n%s", create)
	}
	rows, err := db.Query(`SELECT
media_document_id,
JSON_VALUE(message_data, '$.media.document.id' RETURNING SIGNED NULL ON EMPTY NULL ON ERROR),
media_photo_id,
JSON_VALUE(message_data, '$.media.photo_FLAGPHOTO.id' RETURNING SIGNED NULL ON EMPTY NULL ON ERROR)
FROM messages ORDER BY user_message_box_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var a, b, c, d sql.NullInt64
		if err = rows.Scan(&a, &b, &c, &d); err != nil {
			t.Fatal(err)
		}
		if a != b || c != d {
			t.Errorf("upgraded column (%s,%s) != JSON_VALUE (%s,%s)", nullText(a), nullText(c), nullText(b), nullText(d))
		}
	}
	if got := explainKey(t, db, `EXPLAIN SELECT id FROM messages WHERE media_document_id = 7`); got != "media_document_id" {
		t.Fatalf("legacy upgrade EXPLAIN key %q", got)
	}
}

func applyStartup(t *testing.T, bin, container, dir string) {
	t.Helper()
	execSQL(t, bin, container, "mysql", `CREATE DATABASE IF NOT EXISTS teamgram CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci`)
	mysqlFile(t, bin, container, "teamgram", filepath.Join(dir, "1_teamgram.sql"))
	applyMigrations(t, bin, container, dir)
}

func applyStartupAgain(t *testing.T, bin, container, dir string) {
	t.Helper()
	applyMigrations(t, bin, container, dir)
}

func applyMigrations(t *testing.T, bin, container, dir string) {
	t.Helper()
	mysqlFile(t, bin, container, "teamgram", filepath.Join(dir, "0_migrate_helpers.sql"))
	files, err := filepath.Glob(filepath.Join(dir, "migrate-*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no migrations")
	}
	for _, f := range files {
		mysqlFile(t, bin, container, "teamgram", f)
	}
	mysqlFile(t, bin, container, "teamgram", filepath.Join(dir, "z_init.sql"))
}

type mediaFix struct {
	raw   string
	null  bool
	doc   *int64
	photo *int64
}

func n(v int64) *int64 { return &v }

var mediaFixtures = []mediaFix{
	{raw: `{"media":{"document":{"id":123},"photo_FLAGPHOTO":{"id":42}}}`, doc: n(123), photo: n(42)},
	{raw: `{"media":{"document":{"id":"abc"}}}`},
	{raw: `{"media":{"document":{"id":null}}}`},
	{raw: `{}`},
	{null: true},
	{raw: `{"media":{"document":{"id":true}}}`, doc: n(1)},
	{raw: `{"media":{"document":{"id":false}}}`, doc: n(0)},
	{raw: `{"media":{"document":{"id":1.5}}}`, doc: n(2)},
	{raw: `{"media":{"document":{"id":2.5}}}`, doc: n(2)},
	{raw: `{"media":{"document":{"id":"123"}}}`, doc: n(123)},
	{raw: `{"media":{"document":{"id":"123 "}}}`},
	{raw: `{"media":{"document":{"id":" 123"}}}`, doc: n(123)},
	{raw: `{"media":{"document":{"id":"9223372036854775807"},"photo_FLAGPHOTO":{"id":"9223372036854775807"}}}`, doc: n(9223372036854775807), photo: n(9223372036854775807)},
	{raw: `{"media":{"document":{"id":"-9223372036854775808"},"photo_FLAGPHOTO":{"id":"-9223372036854775808"}}}`, doc: n(-9223372036854775808), photo: n(-9223372036854775808)},
	{raw: `{"media":{"document":{"id":"9223372036854775808"},"photo_FLAGPHOTO":{"id":"9223372036854775808"}}}`},
	{raw: `{"media":{"document":{"id":"-9223372036854775809"},"photo_FLAGPHOTO":{"id":"-9223372036854775809"}}}`},
	// A leading space or tab on a 19-digit value must not make the length
	// check treat it as overflow. TRIM(LEADING ' \t') only deletes that
	// exact two-character prefix, so these used to come back NULL.
	{raw: `{"media":{"document":{"id":" 9223372036854775807"},"photo_FLAGPHOTO":{"id":" 9223372036854775807"}}}`, doc: n(9223372036854775807), photo: n(9223372036854775807)},
	{raw: `{"media":{"document":{"id":" 1000000000000000000"},"photo_FLAGPHOTO":{"id":" 1000000000000000000"}}}`, doc: n(1000000000000000000), photo: n(1000000000000000000)},
	{raw: "{\"media\":{\"document\":{\"id\":\"\\t9223372036854775807\"},\"photo_FLAGPHOTO\":{\"id\":\"\\t9223372036854775807\"}}}", doc: n(9223372036854775807), photo: n(9223372036854775807)},
	{raw: "{\"media\":{\"document\":{\"id\":\" -9223372036854775808\"},\"photo_FLAGPHOTO\":{\"id\":\"\\t-9223372036854775808\"}}}", doc: n(-9223372036854775808), photo: n(-9223372036854775808)},
	{raw: "{\"media\":{\"document\":{\"id\":\" \\t -9223372036854775808\"},\"photo_FLAGPHOTO\":{\"id\":\"\\t -9223372036854775808\"}}}", doc: n(-9223372036854775808), photo: n(-9223372036854775808)},
	{raw: `{"media":{"document":{"id":" 0009223372036854775807"},"photo_FLAGPHOTO":{"id":"+0009223372036854775807"}}}`, doc: n(9223372036854775807), photo: n(9223372036854775807)},
	{raw: "{\"media\":{\"document\":{\"id\":\"\\t-0009223372036854775808\"},\"photo_FLAGPHOTO\":{\"id\":\" \\t+0000000000000000001\"}}}", doc: n(-9223372036854775808), photo: n(1)},
	{raw: `{"media":{"document":{"id":" 9223372036854775808"},"photo_FLAGPHOTO":{"id":"abc"}}}`},
	{raw: `{"media":{"document":{"id":""},"photo_FLAGPHOTO":{"id":""}}}`},
	{raw: `{"media":{"document":{"id":null},"photo_FLAGPHOTO":{"id":null}}}`},
	{raw: `{"media":{"document":{"id":"nope"},"photo_FLAGPHOTO":{"id":"xyz"}}}`},
	{raw: `{"media":{"document":{"id":1e19}}}`},
	{raw: `{"media":{"document":{"id":1e18}}}`, doc: n(1000000000000000000)},
	{raw: `{"media":{"photo_FLAGPHOTO":{"id":"nope"}}}`},
	{raw: `{"media":{"photo_FLAGPHOTO":{"id":7}}}`, photo: n(7)},
	{raw: `{"media":{"document":{"id":0}}}`, doc: n(0)},
}

func nullEq(got sql.NullInt64, want *int64) bool {
	if want == nil {
		return !got.Valid
	}
	return got.Valid && got.Int64 == *want
}

func nullText(v sql.NullInt64) string {
	if !v.Valid {
		return "NULL"
	}
	return strconv.FormatInt(v.Int64, 10)
}

func ptrText(v *int64) string {
	if v == nil {
		return "NULL"
	}
	return strconv.FormatInt(*v, 10)
}

func explainKey(t *testing.T, db *sql.DB, q string) string {
	t.Helper()
	rows, err := db.Query(q)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatal("EXPLAIN returned no row")
	}
	vals := make([]sql.NullString, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err = rows.Scan(ptrs...); err != nil {
		t.Fatal(err)
	}
	for i, name := range cols {
		if strings.EqualFold(name, "key") {
			return vals[i].String
		}
	}
	t.Fatalf("EXPLAIN columns %v", cols)
	return ""
}

func openMySQL(t *testing.T, port, schema string) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("root:root@tcp(127.0.0.1:%s)/%s?parseTime=true&charset=utf8mb4", port, schema)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var last error
	for i := 0; i < 30; i++ {
		last = db.Ping()
		if last == nil {
			return db
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("ping %s: %v", dsn, last)
	return nil
}

func startMySQL(t *testing.T, bin, image, name string) string {
	t.Helper()
	_ = runDocker(t, bin, "rm", "-f", name)
	out, err := runDockerOutput(t, bin, "run", "-d", "--name", name,
		"-e", "MYSQL_ROOT_PASSWORD=root",
		"-e", "MYSQL_ROOT_HOST=%",
		"-p", "127.0.0.1::3306",
		image)
	if err != nil {
		t.Fatalf("start %s: %v\n%s", image, err, out)
	}
	t.Cleanup(func() { _ = runDocker(t, bin, "rm", "-f", name) })
	// The image starts a temporary server to initialize the datadir, and that
	// server answers SELECT 1 before the entrypoint kills it. The next client
	// then gets ERROR 2013 (errno 104) at "reading initial communication
	// packet". Wait until init has finished, then require two successful
	// queries a second apart so the real server is the one that answered.
	deadline := time.Now().Add(3 * time.Minute)
	var last []byte
	initDone := false
	stable := 0
	for time.Now().Before(deadline) {
		if !initDone {
			logs, _ := runDockerOutput(t, bin, "logs", name)
			last = logs
			if strings.Contains(string(logs), "MySQL init process done. Ready for start up.") {
				initDone = true
			} else {
				time.Sleep(time.Second)
				continue
			}
		}
		out, err = runDockerOutput(t, bin, "exec", name, "mysql", "-uroot", "-proot", "-N", "-e", "SELECT 1")
		if err == nil && strings.Contains(string(out), "1") {
			stable++
			if stable >= 2 {
				portOut, perr := runDockerOutput(t, bin, "port", name, "3306")
				if perr != nil {
					t.Fatal(perr)
				}
				// 127.0.0.1:32768
				fields := strings.Fields(string(portOut))
				if len(fields) == 0 {
					t.Fatalf("port output %q", portOut)
				}
				hostport := strings.TrimSpace(fields[len(fields)-1])
				port := hostport[strings.LastIndex(hostport, ":")+1:]
				if port == "" || port == hostport {
					t.Fatalf("port output %q", portOut)
				}
				return port
			}
			time.Sleep(time.Second)
			continue
		}
		stable = 0
		last = out
		time.Sleep(time.Second)
	}
	t.Fatalf("%s did not accept queries: %s", image, last)
	return ""
}

func mysqlFile(t *testing.T, bin, container, schema, path string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cmd := dockerCmd(bin, "exec", "-i", container, "mysql", "-uroot", "-proot", schema)
	cmd.Stdin = f
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", filepath.Base(path), err, out)
	}
}

func execSQL(t *testing.T, bin, container, schema, query string) {
	t.Helper()
	out, err := runDockerOutput(t, bin, "exec", "-i", container, "mysql", "-uroot", "-proot", schema, "-e", query)
	if err != nil {
		t.Fatalf("sql: %v\n%s\n%s", err, query, out)
	}
}

func dockerBin(t *testing.T) string {
	t.Helper()
	if exec.Command("docker", "info").Run() == nil {
		return "docker"
	}
	if exec.Command("sudo", "docker", "info").Run() == nil {
		return "sudo"
	}
	t.Fatal("docker is not available")
	return ""
}

func dockerCmd(bin string, args ...string) *exec.Cmd {
	if bin == "sudo" {
		return exec.Command("sudo", append([]string{"docker"}, args...)...)
	}
	return exec.Command("docker", args...)
}

func runDocker(t *testing.T, bin string, args ...string) error {
	t.Helper()
	return dockerCmd(bin, args...).Run()
}

func runDockerOutput(t *testing.T, bin string, args ...string) ([]byte, error) {
	t.Helper()
	return dockerCmd(bin, args...).CombinedOutput()
}
