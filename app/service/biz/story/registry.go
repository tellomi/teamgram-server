package story

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/teamgram-server/app/service/biz/channel"

	_ "github.com/go-sql-driver/mysql"
)

var (
	regMu  sync.Mutex
	shared *Service
)

func init() {
	channel.LookupStories = func(ctx context.Context, viewer, channelID int64) channel.StoryAttach {
		return Shared().AttachChannel(ctx, viewer, channelID)
	}
}

// Shared is the process-wide story service. BFF handlers and channel object
// rendering both read it, so they must share one store.
func Shared() *Service {
	regMu.Lock()
	defer regMu.Unlock()
	if shared == nil {
		shared = New(NewMemoryStore())
	}
	return shared
}

// Use replaces the process-wide service. Hooks already installed are kept
// when the replacement has none.
func Use(s *Service) {
	regMu.Lock()
	defer regMu.Unlock()
	if shared != nil && s != nil && s.hooksEmpty() {
		s.SetHooks(shared.hooksCopy())
	}
	shared = s
}

// UseDSN opens MySQL when dsn or TEAMGRAM_MYSQL_DSN is set.
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
	Use(svc)
	return nil
}

// OpenMySQL connects, creates the story tables, and returns a service.
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
	return New(newMySQLStore(db)), nil
}

func ensureSchema(ctx context.Context, db *sql.DB) error {
	for _, stmt := range strings.Split(schemaSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}
