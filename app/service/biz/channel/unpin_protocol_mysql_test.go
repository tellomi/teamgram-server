package channel

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// TestUnpinAllProtocolMySQL runs the same pin protocol as TestUnpinAllProtocol
// through channel.OpenMySQL. Each process gets its own database.
func TestUnpinAllProtocolMySQL(t *testing.T) {
	svc := openUnpinMySQL(t)
	checkUnpinProtocol(t, svc, context.Background())
}

func openUnpinMySQL(t *testing.T) *Service {
	t.Helper()
	admin, dsn := openUnpinAdmin(t)
	schema := fmt.Sprintf("unpin_ch_%d_%d", os.Getpid(), time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE `" + schema + "`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS `" + schema + "`") })
	svc, err := OpenMySQL(withSchema(dsn, schema))
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func openUnpinAdmin(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dsn := os.Getenv("TEAMGRAM_MYSQL_DSN")
	if dsn == "" {
		dsn = "root:@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true&multiStatements=true"
	}
	admin, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("mysql not available: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	admin.SetMaxOpenConns(2)
	if err = admin.Ping(); err != nil {
		t.Skipf("mysql not available: %v", err)
	}
	return admin, dsn
}

func withSchema(dsn, schema string) string {
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
