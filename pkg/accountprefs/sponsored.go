// Package accountprefs stores account switches that are not fields of
// globalPrivacySettings. The sponsored-message switch is one of them:
// users.getFullUser reads sponsored_enabled, and the privacy row is the
// durable copy.
package accountprefs

import (
	"context"
	"database/sql"
	"os"
	"sync"

	"github.com/teamgram/teamgram-server/pkg/mysqlpool"
	"github.com/teamgram/teamgram-server/pkg/mysqltls"

	_ "github.com/go-sql-driver/mysql"
)

var (
	mu   sync.Mutex
	mem  = map[int64]bool{}
	db   *sql.DB
	open sync.Once
)

// UseDSN opens the teamgram database. An empty dsn keeps the process map.
// TEAMGRAM_MYSQL_DSN overrides dsn. The table is created by
// migrate-20261114-privacy-gifts.sql; this writer only updates the column.
func UseDSN(dsn string) error {
	if env := os.Getenv("TEAMGRAM_MYSQL_DSN"); env != "" {
		dsn = env
	}
	if dsn == "" {
		return nil
	}
	conn, err := mysqltls.Open(dsn)
	if err != nil {
		return err
	}
	if err = conn.Ping(); err != nil {
		_ = mysqlpool.Discard(conn)
		return err
	}
	mu.Lock()
	if db != nil {
		_ = mysqlpool.Discard(db)
	}
	db = conn
	mu.Unlock()
	return nil
}

// SetSponsored records whether this user wants sponsored messages.
func SetSponsored(ctx context.Context, userID int64, enabled bool) error {
	if userID == 0 {
		return nil
	}
	mu.Lock()
	mem[userID] = enabled
	conn := db
	mu.Unlock()
	if conn == nil {
		return nil
	}
	_, err := conn.ExecContext(ctx, `insert into user_global_privacy_settings (user_id, sponsored_enabled) values (?, ?)
		on duplicate key update sponsored_enabled = values(sponsored_enabled)`, userID, enabled)
	return err
}

// Sponsored reports the stored switch. A user who never toggled it is false,
// which is what users.getFullUser returned before the switch was stored.
func Sponsored(ctx context.Context, userID int64) bool {
	if userID == 0 {
		return false
	}
	mu.Lock()
	conn := db
	if conn == nil {
		on := mem[userID]
		mu.Unlock()
		return on
	}
	mu.Unlock()
	var on bool
	err := conn.QueryRowContext(ctx, `select sponsored_enabled from user_global_privacy_settings where user_id = ?`, userID).Scan(&on)
	if err != nil {
		mu.Lock()
		on = mem[userID]
		mu.Unlock()
	}
	return on
}
