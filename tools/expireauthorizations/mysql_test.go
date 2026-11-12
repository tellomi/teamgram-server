package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func TestExpireAuthorizationsDryRunApplyRollback(t *testing.T) {
	dsn, err := expireTestDSN(t)
	if err != nil {
		if os.Getenv("GITHUB_ACTIONS") == "1" {
			t.Fatalf("mysql is required in CI: %v", err)
		}
		t.Skipf("mysql not available: %v", err)
	}
	admin, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schema := fmt.Sprintf("ttl_expire_%d", os.Getpid())
	if _, err := admin.Exec("CREATE DATABASE " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS " + schema) })
	db, err := sql.Open("mysql", dsnWithSchema(dsn, schema))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range strings.Split(expireFixtureSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().Unix()
	if _, err := db.Exec(`INSERT INTO users (id, authorization_ttl_days) VALUES (1, 30)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO auth_users (id, auth_key_id, user_id, hash, date_created, date_active, activity_trusted, deleted) VALUES
		(10, 100, 1, 5, 1000, ?, 0, 0),
		(11, 101, 1, 6, 1000, ?, 1, 0),
		(12, 102, 1, 7, 1000, ?, 1, 0)`,
		now-40*24*60*60, now-40*24*60*60, now-24*60*60); err != nil {
		t.Fatal(err)
	}
	const staleToken = "stale-device-token"
	const liveToken = "live-device-token"
	if _, err := db.Exec(`INSERT INTO devices (id, auth_key_id, user_id, token_type, token, secret, other_uids, lang_code) VALUES
		(1, 100, 1, 1, 'legacy-token', '', '', ''),
		(2, 101, 1, 2, ?, 'abc', '', 'zh'),
		(3, 102, 1, 2, ?, '', '', '')`, staleToken, liveToken); err != nil {
		t.Fatal(err)
	}

	n, err := countStale(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("dry-run count %d, want 1 (trusted idle only)", n)
	}
	var deleted int
	if err := db.QueryRow(`SELECT deleted FROM auth_users WHERE id = 11`).Scan(&deleted); err != nil {
		t.Fatal(err)
	}
	if deleted != 0 {
		t.Fatal("dry-run wrote auth_users")
	}

	runID, copied, err := applyRun(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if copied != 1 || runID == 0 {
		t.Fatalf("apply run %d copied %d", runID, copied)
	}
	assertAuth(t, db, 10, 0, 0)
	assertAuth(t, db, 11, 1, 1)
	assertAuth(t, db, 12, 0, 1)
	if c := tokenCount(t, db, 101); c != 0 {
		t.Fatalf("stale tokens left %d", c)
	}
	if c := tokenCount(t, db, 100); c != 1 {
		t.Fatalf("legacy token %d", c)
	}
	if c := tokenCount(t, db, 102); c != 1 {
		t.Fatalf("live token %d", c)
	}

	restored, err := rollbackRun(ctx, db, runID)
	if err != nil {
		t.Fatal(err)
	}
	if restored != 1 {
		t.Fatalf("restored %d", restored)
	}
	assertAuth(t, db, 11, 0, 1)
	var active int64
	var token string
	if err := db.QueryRow(`SELECT date_active FROM auth_users WHERE id = 11`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != now-40*24*60*60 {
		t.Fatalf("date_active %d", active)
	}
	if err := db.QueryRow(`SELECT token FROM devices WHERE auth_key_id = 101 AND user_id = 1`).Scan(&token); err != nil {
		t.Fatal(err)
	}
	if token != staleToken {
		t.Fatal("rollback did not restore the device token")
	}
}

func assertAuth(t *testing.T, db *sql.DB, id int64, deleted, trusted int) {
	t.Helper()
	var gotDeleted, gotTrusted int
	if err := db.QueryRow(`SELECT deleted, activity_trusted FROM auth_users WHERE id = ?`, id).Scan(&gotDeleted, &gotTrusted); err != nil {
		t.Fatal(err)
	}
	if gotDeleted != deleted || gotTrusted != trusted {
		t.Fatalf("auth %d deleted %d trusted %d, want %d %d", id, gotDeleted, gotTrusted, deleted, trusted)
	}
}

func tokenCount(t *testing.T, db *sql.DB, authKeyID int64) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM devices WHERE auth_key_id = ?`, authKeyID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

const expireFixtureSQL = `
CREATE TABLE users (
  id BIGINT NOT NULL,
  authorization_ttl_days INT NOT NULL DEFAULT 180,
  PRIMARY KEY (id)
) ENGINE=InnoDB;
CREATE TABLE auth_users (
  id BIGINT NOT NULL,
  auth_key_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  hash BIGINT NOT NULL DEFAULT 0,
  date_created BIGINT NOT NULL DEFAULT 0,
  date_active BIGINT NOT NULL DEFAULT 0,
  activity_trusted TINYINT NOT NULL DEFAULT 0,
  deleted TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (id)
) ENGINE=InnoDB;
CREATE TABLE devices (
  id BIGINT NOT NULL,
  auth_key_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  token_type INT NOT NULL,
  token VARCHAR(512) NOT NULL,
  no_muted TINYINT NOT NULL DEFAULT 0,
  locked_period INT NOT NULL DEFAULT 0,
  app_sandbox TINYINT NOT NULL DEFAULT 0,
  secret VARCHAR(1024) NOT NULL DEFAULT '',
  other_uids VARCHAR(1024) NOT NULL DEFAULT '',
  lang_code VARCHAR(32) NOT NULL DEFAULT '',
  state TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (id)
) ENGINE=InnoDB`

func expireTestDSN(t *testing.T) (string, error) {
	t.Helper()
	if dsn := strings.TrimSpace(os.Getenv("TEAMGRAM_MYSQL_DSN")); dsn != "" && pingMySQL(dsn) {
		return dsn, nil
	}
	candidates := []string{
		"teamgram:teamgram@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true",
		"root:root@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true",
		"root:@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true",
	}
	for _, dsn := range candidates {
		if pingMySQL(dsn) {
			return dsn, nil
		}
	}
	if port, err := startExpireMySQL(t); err == nil {
		dsn := fmt.Sprintf("root:root@tcp(127.0.0.1:%s)/mysql?charset=utf8mb4&parseTime=true", port)
		if pingMySQL(dsn) {
			return dsn, nil
		}
	}
	return "", fmt.Errorf("no mysql")
}

func pingMySQL(dsn string) bool {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return false
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return db.PingContext(ctx) == nil
}

func dsnWithSchema(dsn, schema string) string {
	slash := strings.LastIndex(dsn, "/")
	if slash < 0 {
		return dsn + "/" + schema
	}
	rest := dsn[slash+1:]
	if q := strings.Index(rest, "?"); q >= 0 {
		return dsn[:slash+1] + schema + rest[q:]
	}
	return dsn[:slash+1] + schema
}

func startExpireMySQL(t *testing.T) (string, error) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		return "", err
	}
	name := fmt.Sprintf("teamgram-ttl-expire-%d", os.Getpid())
	_ = exec.Command("docker", "rm", "-f", name).Run()
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	out, err := exec.Command("docker", "run", "-d", "--name", name,
		"-e", "MYSQL_ROOT_PASSWORD=root",
		"-e", "MYSQL_ROOT_HOST=%",
		"-p", "127.0.0.1::3306",
		"mysql:8.0.46").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker run: %w %s", err, out)
	}
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		probe, probeErr := exec.Command("docker", "exec", name, "mysql", "-uroot", "-proot", "-N", "-e", "SELECT 1").CombinedOutput()
		if probeErr == nil && strings.Contains(string(probe), "1") {
			portOut, portErr := exec.Command("docker", "port", name, "3306").CombinedOutput()
			if portErr != nil {
				return "", portErr
			}
			fields := strings.Fields(string(portOut))
			if len(fields) == 0 {
				return "", fmt.Errorf("docker port %q", portOut)
			}
			hostport := fields[len(fields)-1]
			return hostport[strings.LastIndex(hostport, ":")+1:], nil
		}
		time.Sleep(time.Second)
	}
	return "", fmt.Errorf("mysql container did not become ready")
}
