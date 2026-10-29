package reaction

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var (
	regMu  sync.Mutex
	shared Store = NewMemory()
)

// Shared is the process-wide store for private and basic-group reactions.
// Channel messages use the channel service's own store, which is this same
// MySQL table when a DSN is configured.
func Shared() Store {
	regMu.Lock()
	defer regMu.Unlock()
	if shared == nil {
		shared = NewMemory()
	}
	return shared
}

// Use replaces the process-wide store. Tests pass a fresh memory store.
func Use(s Store) {
	regMu.Lock()
	shared = s
	regMu.Unlock()
}

// UseDSN opens message_reactions on teamgram MySQL. Empty dsn keeps memory.
func UseDSN(dsn string) error {
	if env := os.Getenv("TEAMGRAM_MYSQL_DSN"); env != "" {
		dsn = env
	}
	if dsn == "" {
		return nil
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return err
	}
	if err = EnsureSchema(ctx, db); err != nil {
		_ = db.Close()
		return err
	}
	Use(NewMySQL(db))
	return nil
}

func timeNow() int64 { return time.Now().Unix() }
