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
			if srv.jsonFunc {
				checkLegacyFunctionalIndex(t, bin, srv.name, port, dir)
			}
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
	{raw: `{"media":{"document":{"id":"9223372036854775807"}}}`, doc: n(9223372036854775807)},
	{raw: `{"media":{"document":{"id":"9223372036854775808"}}}`},
	{raw: `{"media":{"document":{"id":"-9223372036854775808"}}}`, doc: n(-9223372036854775808)},
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
	deadline := time.Now().Add(2 * time.Minute)
	var last []byte
	for time.Now().Before(deadline) {
		out, err = runDockerOutput(t, bin, "exec", name, "mysql", "-uroot", "-proot", "-N", "-e", "SELECT 1")
		if err == nil && strings.Contains(string(out), "1") {
			portOut, err := runDockerOutput(t, bin, "port", name, "3306")
			if err != nil {
				t.Fatal(err)
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
