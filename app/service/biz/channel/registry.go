package channel

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"time"

	"github.com/teamgram/teamgram-server/app/service/biz/reaction"

	_ "github.com/go-sql-driver/mysql"
)

var (
	regMu  sync.Mutex
	shared *Service
)

// Shared is the process-wide channel service. BFF and the msg service must
// open the same MySQL DSN or they will not see each other's channels.
func Shared() *Service {
	regMu.Lock()
	defer regMu.Unlock()
	if shared == nil {
		shared = New(NewMemoryStore())
	}
	return shared
}

// Use replaces the process-wide service.
func Use(s *Service) {
	regMu.Lock()
	shared = s
	regMu.Unlock()
}

// UseDSN opens MySQL when dsn or TEAMGRAM_MYSQL_DSN is set and installs it
// as the shared service. Hooks already registered are kept.
func UseDSN(dsn string) error {
	if env := os.Getenv("TEAMGRAM_MYSQL_DSN"); env != "" {
		dsn = env
	}
	if dsn == "" {
		return nil
	}
	svc, err := OpenMySQL(dsn)
	if err != nil {
		return err
	}
	prev := Shared()
	svc.SetHooks(prev.hooks())
	Use(svc)
	return nil
}

// OpenMySQL connects, creates the channel tables, and returns a service.
func OpenMySQL(dsn string) (*Service, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(32)
	db.SetMaxIdleConns(8)
	db.SetConnMaxLifetime(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err = ensureSchema(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err = reaction.EnsureSchema(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	svc := New(newMySQLStore(db))
	svc.reactions = reaction.NewMySQL(db)
	return svc, nil
}
