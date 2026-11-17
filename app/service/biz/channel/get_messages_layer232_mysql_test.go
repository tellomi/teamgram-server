package channel

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Same read as the memory store, through the MySQL channel tables. The post
// is inserted by Service.Send with no TL predicate. inbox-mysql sets
// TEAMGRAM_REQUIRE_INBOX_MYSQL, so a missing database fails the job.
func TestChannelGetMessagesLayer232MySQL(t *testing.T) {
	dsn := channelMySQLDSN(t)
	admin, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	if err = admin.Ping(); err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("chan_l232_%d_%d", os.Getpid(), time.Now().UnixNano())
	if _, err = admin.Exec("CREATE DATABASE `" + schema + "`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS `" + schema + "`") })
	svc, err := OpenMySQL(channelSchemaDSN(dsn, schema))
	if err != nil {
		t.Fatal(err)
	}
	runLayer232StoredPost(t, svc)
}

func channelMySQLDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEAMGRAM_MYSQL_DSN")
	required := os.Getenv("TEAMGRAM_REQUIRE_INBOX_MYSQL") == "1"
	if dsn == "" {
		if required {
			t.Fatal("TEAMGRAM_MYSQL_DSN is required")
		}
		t.Skip("TEAMGRAM_MYSQL_DSN is not set")
	}
	return dsn
}

func channelSchemaDSN(dsn, schema string) string {
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
