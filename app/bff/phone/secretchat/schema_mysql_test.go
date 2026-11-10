package secretchat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Local throwaway server only. TELLOMI_SCHEMA_TEST_DSN is a CI fixture
// (the compose MySQL, root/root). These tests do not read TEAMGRAM_MYSQL_DSN.
// Each test creates its own database and drops only that database.

var schemaSeq atomic.Int64

func TestSchemaSplitsIntoCreateStatements(t *testing.T) {
	n := 0
	for _, stmt := range strings.Split(schema, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		n++
		if !strings.HasPrefix(stmt, "CREATE") {
			t.Fatalf("split piece does not start with CREATE (semicolon inside a literal or comment?):\n%s", stmt)
		}
	}
	if n != 5 {
		t.Fatalf("statements = %d, want 5", n)
	}
}

func TestSecretQtsSchemaMySQL(t *testing.T) {
	dsn := openSchemaDSN(t)
	db := openMySQL(t, dsn)
	_, err := db.Exec(schema)
	if !syntaxAtSecretQts(err) {
		t.Fatalf("one Exec of the startup script = %v, want MySQL 1064 near secret_qts_state", err)
	}
	if err = openAndClose(t, dsn); err != nil {
		t.Fatal(err)
	}
	if err = openAndClose(t, dsn); err != nil {
		t.Fatalf("second open: %v", err)
	}
	assertSecretTables(t, db)
}

func TestSecretQtsMigrationMySQL(t *testing.T) {
	db := openMySQL(t, openSchemaDSN(t))
	script := secretQtsMigration(t)
	if err := execScript(db, script); err != nil {
		t.Fatal(err)
	}
	if err := execScript(db, script); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	assertSecretTables(t, db)
}

func openAndClose(t *testing.T, dsn string) error {
	t.Helper()
	be, err := openSQL(dsn)
	if err != nil {
		return err
	}
	t.Cleanup(func() { _ = be.db.Close() })
	return nil
}

func assertSecretTables(t *testing.T, db *sql.DB) {
	t.Helper()
	var n int
	err := db.QueryRow(`
SELECT COUNT(*) FROM information_schema.tables
WHERE table_schema = DATABASE()
  AND table_name IN ('secret_chats', 'secret_qts_state', 'secret_qts', 'secret_sent', 'secret_files')`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("secret tables = %d, want 5", n)
	}
	if _, err = db.Exec(`INSERT INTO secret_qts_state (user_id, qts) VALUES (7, 1)`); err != nil {
		t.Fatal(err)
	}
	var qts int
	if err = db.QueryRow(`SELECT qts FROM secret_qts_state WHERE user_id = 7`).Scan(&qts); err != nil {
		t.Fatal(err)
	}
	if qts != 1 {
		t.Fatalf("qts = %d, want 1", qts)
	}
}

func syntaxAtSecretQts(err error) bool {
	if err == nil {
		return false
	}
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return me.Number == 1064 && strings.Contains(me.Message, "secret_qts_state")
	}
	msg := err.Error()
	return strings.Contains(msg, "1064") && strings.Contains(msg, "secret_qts_state")
}

func secretQtsMigration(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		path := filepath.Join(dir, "teamgramd", "deploy", "sql", "migrate-20261120-secret-qts.sql")
		raw, err := os.ReadFile(path)
		if err == nil {
			return string(raw)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("migrate-20261120-secret-qts.sql not found")
		}
		dir = parent
	}
}

func openMySQL(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}

func openSchemaDSN(t *testing.T) string {
	t.Helper()
	required := strings.TrimSpace(os.Getenv("TELLOMI_SCHEMA_TEST_DSN")) != ""
	deadline := time.Now().Add(2 * time.Second)
	if required {
		deadline = time.Now().Add(30 * time.Second)
	}
	var last error
	for {
		for _, dsn := range schemaCandidates() {
			admin, err := sql.Open("mysql", dsn)
			if err != nil {
				last = err
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err = admin.PingContext(ctx)
			cancel()
			if err != nil {
				_ = admin.Close()
				last = err
				continue
			}
			name := fmt.Sprintf("sqts_%d_%d", os.Getpid(), schemaSeq.Add(1))
			ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
			_, err = admin.ExecContext(ctx, "CREATE DATABASE `"+name+"`")
			cancel()
			if err != nil {
				_ = admin.Close()
				last = err
				continue
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_, _ = admin.ExecContext(ctx, "DROP DATABASE `"+name+"`")
				_ = admin.Close()
			})
			return swapDB(dsn, name)
		}
		if !required || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Second)
	}
	if required {
		t.Fatalf("schema mysql: %v", last)
	}
	t.Skipf("mysql not available: %v", last)
	return ""
}

func schemaCandidates() []string {
	if dsn := strings.TrimSpace(os.Getenv("TELLOMI_SCHEMA_TEST_DSN")); dsn != "" {
		return []string{dsn}
	}
	// Fixture logins for a local throwaway server. Not production credentials.
	return []string{
		"root:root@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true",
		"root:@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true",
		"teamgram:teamgram@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true",
	}
}

func swapDB(dsn, name string) string {
	slash := strings.LastIndex(dsn, "/")
	if slash < 0 {
		return dsn + "/" + name
	}
	rest := dsn[slash+1:]
	query := ""
	if i := strings.Index(rest, "?"); i >= 0 {
		query = rest[i:]
	}
	return dsn[:slash+1] + name + stripMulti(query)
}

func stripMulti(query string) string {
	query = strings.TrimPrefix(query, "?")
	if query == "" {
		return ""
	}
	var kept []string
	for _, part := range strings.Split(query, "&") {
		if part == "" || strings.HasPrefix(strings.ToLower(part), "multistatements=") {
			continue
		}
		kept = append(kept, part)
	}
	if len(kept) == 0 {
		return ""
	}
	return "?" + strings.Join(kept, "&")
}
