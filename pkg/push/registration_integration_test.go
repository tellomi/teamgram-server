package push

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

// This suite only accepts an explicitly configured loopback MySQL. It creates
// its own uniquely named database, never drops a database that already existed,
// and cannot silently skip when the dedicated CI check requires MySQL.
func openRegistrationIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	required := os.Getenv("TEAMGRAM_REQUIRE_PUSH_MYSQL") == "1"
	unavailable := func() {
		t.Helper()
		if required {
			t.Fatal("required local MySQL integration service unavailable")
		}
		t.Skip("set TEAMGRAM_MYSQL_DSN to a disposable loopback MySQL service")
	}
	dsn := os.Getenv("TEAMGRAM_MYSQL_DSN")
	if dsn == "" {
		unavailable()
	}
	cfg, err := mysqlDriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid test MySQL DSN")
	}
	host, _, err := net.SplitHostPort(cfg.Addr)
	ip := net.ParseIP(host)
	if cfg.Net != "tcp" || err != nil || (host != "localhost" && (ip == nil || !ip.IsLoopback())) {
		t.Fatal("registration integration tests require a loopback TCP MySQL service")
	}
	cfg.Timeout = 2 * time.Second
	cfg.ReadTimeout = 10 * time.Second
	cfg.WriteTimeout = 10 * time.Second
	cfg.DBName = "mysql"
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("cannot open test MySQL service")
	}
	t.Cleanup(func() { admin.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = admin.PingContext(ctx); err != nil {
		unavailable()
	}
	schema := fmt.Sprintf("teamgram_push_registration_%d", time.Now().UnixNano())
	if _, err = admin.Exec("CREATE DATABASE " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE " + schema); err != nil {
			t.Errorf("clean integration database: %v", err)
		}
	})
	cfg.DBName = schema
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("cannot open isolated test database")
	}
	db.SetMaxOpenConns(8)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE devices (
      id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
      auth_key_id BIGINT NOT NULL, user_id BIGINT NOT NULL,
      token_type INT NOT NULL,
      token VARCHAR(512) COLLATE utf8mb4_unicode_ci NOT NULL,
      no_muted TINYINT NOT NULL DEFAULT 0, locked_period INT NOT NULL DEFAULT 0,
      app_sandbox TINYINT NOT NULL DEFAULT 0,
      secret VARCHAR(1024) NOT NULL DEFAULT '', other_uids VARCHAR(1024) NOT NULL DEFAULT '',
      lang_code VARCHAR(32) NOT NULL DEFAULT '', state TINYINT NOT NULL DEFAULT 0,
      created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
      updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
      UNIQUE KEY auth_key_id (auth_key_id, user_id, token_type)
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		t.Fatal(err)
	}
	// Execute the actual helper and migration, not a test reimplementation.
	helpers, err := os.ReadFile("../../teamgramd/deploy/sql/0_migrate_helpers.sql")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(helpers), "CREATE PROCEDURE teamgram_add_index(")
	if start < 0 {
		t.Fatal("index helper not found")
	}
	proc := string(helpers)[start:]
	end := strings.Index(proc, "END$$")
	if end < 0 {
		t.Fatal("index helper terminator not found")
	}
	if _, err := db.Exec(proc[:end+3]); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../teamgramd/deploy/sql/migrate-20261111-push-token-owner.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Seed a registration before DDL and ensure two executions preserve it.
	if _, err := db.Exec(`INSERT INTO devices (auth_key_id,user_id,token_type,token) VALUES (991,991,2,'migration-fixture')`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := db.Exec(string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	var cols string
	var nonUnique int
	if err := db.QueryRow(`SELECT GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX), MIN(NON_UNIQUE)
        FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE()
        AND TABLE_NAME='devices' AND INDEX_NAME='idx_devices_push_token'`).Scan(&cols, &nonUnique); err != nil {
		t.Fatal(err)
	}
	if cols != "token_type,token" || nonUnique != 1 {
		t.Fatalf("wrong migration index shape: %q nonUnique=%d", cols, nonUnique)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM devices WHERE user_id=991 AND token='migration-fixture'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("migration changed registration: count=%d err=%v", count, err)
	}
	if _, err := db.Exec(`DELETE FROM devices WHERE user_id=991`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestRegistrationMySQLIntegrationTransfer(t *testing.T) {
	db := openRegistrationIntegrationDB(t)
	store := &MySQL{db: db}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	fixtures := []Device{
		{AuthKeyID: 10, UserID: 10, TokenType: TokenFCM, Token: "SharedToken"},
		{AuthKeyID: 20, UserID: 20, TokenType: TokenFCM, Token: "SharedToken", OtherUIDs: []int64{10}},
		{AuthKeyID: 30, UserID: 30, TokenType: TokenFCM, Token: "sharedtoken"},
		{AuthKeyID: 40, UserID: 40, TokenType: TokenAPNS, Token: "SharedToken"},
		{AuthKeyID: 50, UserID: 50, TokenType: TokenFCM, Token: "unrelated"},
	}
	for _, d := range fixtures {
		if err := store.Upsert(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM devices WHERE token_type=2 AND BINARY token=BINARY 'SharedToken'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("declared multi-account not preserved: count=%d err=%v", count, err)
	}
	next := Device{AuthKeyID: 21, UserID: 20, TokenType: TokenFCM, Token: "SharedToken"}
	for i := 0; i < 2; i++ {
		if err := store.Upsert(ctx, next); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := db.Query(`SELECT auth_key_id, user_id FROM devices ORDER BY user_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[int64]int64{}
	for rows.Next() {
		var auth, user int64
		if err := rows.Scan(&auth, &user); err != nil {
			t.Fatal(err)
		}
		if _, exists := got[user]; exists {
			t.Fatal("duplicate owner registration")
		}
		got[user] = auth
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[20] != 21 || got[30] != 30 || got[40] != 40 || got[50] != 50 {
		t.Fatalf("transfer affected wrong owners: %v", got)
	}
}

func TestRegistrationMySQLIntegrationConcurrentTransfer(t *testing.T) {
	db := openRegistrationIntegrationDB(t)
	store := &MySQL{db: db}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for round := 0; round < 20; round++ {
		token := fmt.Sprintf("concurrent-%d", round)
		// Cover both first-registration gap locks and existing-token transfer.
		if round%2 == 0 {
			if err := store.Upsert(ctx, Device{AuthKeyID: 100, UserID: 100, TokenType: TokenFCM, Token: token}); err != nil {
				t.Fatal(err)
			}
		}
		start := make(chan struct{})
		done := make(chan error, 2)
		for _, uid := range []int64{201, 202} {
			go func(uid int64) {
				<-start
				done <- store.Upsert(ctx, Device{AuthKeyID: uid, UserID: uid, TokenType: TokenFCM, Token: token})
			}(uid)
		}
		close(start)
		for i := 0; i < 2; i++ {
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		}
		var count int
		var owner int64
		if err := db.QueryRow(`SELECT COUNT(*), COALESCE(MIN(user_id),0) FROM devices WHERE token_type=2 AND BINARY token=BINARY ?`, token).Scan(&count, &owner); err != nil {
			t.Fatal(err)
		}
		if count != 1 || (owner != 201 && owner != 202) {
			t.Fatalf("round %d has conflicting registration owners: count=%d owner=%d", round, count, owner)
		}
		// Avoid reusing the same auth/user unique key across rounds.
		if _, err := db.Exec(`DELETE FROM devices`); err != nil {
			t.Fatal(err)
		}
	}
}
