// Package reactionnotify stores account reaction-notification settings per user.
//
// user_reactions_notify.settings_blob is a layer 232 reactionsNotifySettings.
// teamgramd/deploy/migrate.sh applies
// teamgramd/deploy/sql/migrate-20261029-reaction-notify.sql. The process
// also creates the table on startup. TEAMGRAM_MYSQL_DSN overrides the DSN.
package reactionnotify

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"

	_ "github.com/go-sql-driver/mysql"
	"github.com/zeromicro/go-zero/core/logx"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS user_reactions_notify (
  user_id BIGINT NOT NULL,
  settings_blob MEDIUMBLOB NOT NULL,
  PRIMARY KEY (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

// ErrNotFound is returned when the user has never saved settings.
var ErrNotFound = errors.New("reaction notify settings not found")

// Store is the per-user reactionsNotifySettings blob.
type Store interface {
	Get(ctx context.Context, userID int64) (*mtproto.ReactionsNotifySettings, error)
	Put(ctx context.Context, userID int64, settings *mtproto.ReactionsNotifySettings) error
}

type memoryStore struct {
	mu   sync.Mutex
	rows map[int64][]byte
}

func newMemoryStore() *memoryStore {
	return &memoryStore{rows: map[int64][]byte{}}
}

func (m *memoryStore) Get(_ context.Context, userID int64) (*mtproto.ReactionsNotifySettings, error) {
	m.mu.Lock()
	blob := append([]byte(nil), m.rows[userID]...)
	m.mu.Unlock()
	if blob == nil {
		return nil, ErrNotFound
	}
	return Decode(blob)
}

func (m *memoryStore) Put(_ context.Context, userID int64, settings *mtproto.ReactionsNotifySettings) error {
	blob, err := Encode(settings)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.rows[userID] = blob
	m.mu.Unlock()
	return nil
}

type mysqlStore struct {
	db *sql.DB
}

func openStore(dsn string) Store {
	dsn = strings.TrimSpace(dsn)
	if env := strings.TrimSpace(os.Getenv("TEAMGRAM_MYSQL_DSN")); env != "" {
		dsn = env
	}
	if dsn == "" {
		return newMemoryStore()
	}
	st, err := newMySQLStore(dsn)
	if err != nil {
		logx.Errorf("reactionnotify: mysql unavailable, settings stay in memory until restart: %v", err)
		return newMemoryStore()
	}
	return st
}

func newMySQLStore(dsn string) (*mysqlStore, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	stmt := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(schemaSQL), ";"))
	if _, err = db.ExecContext(ctx, stmt); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &mysqlStore{db: db}, nil
}

func (s *mysqlStore) Get(ctx context.Context, userID int64) (*mtproto.ReactionsNotifySettings, error) {
	var blob []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT settings_blob FROM user_reactions_notify WHERE user_id=?`, userID).Scan(&blob)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return Decode(blob)
}

func (s *mysqlStore) Put(ctx context.Context, userID int64, settings *mtproto.ReactionsNotifySettings) error {
	blob, err := Encode(settings)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO user_reactions_notify (user_id, settings_blob) VALUES (?, ?)
		 ON DUPLICATE KEY UPDATE settings_blob=VALUES(settings_blob)`,
		userID, blob)
	return err
}

var (
	mu      sync.RWMutex
	current Store = newMemoryStore()
)

// Open stores the process-wide settings. An empty dsn uses memory unless
// TEAMGRAM_MYSQL_DSN is set.
func Open(dsn string) Store {
	st := openStore(dsn)
	mu.Lock()
	current = st
	mu.Unlock()
	return st
}

// Use replaces the process-wide store. Tests use it to isolate rows.
func Use(st Store) {
	if st == nil {
		st = newMemoryStore()
	}
	mu.Lock()
	current = st
	mu.Unlock()
}

// Current is the process-wide store.
func Current() Store {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Encode serializes reactionsNotifySettings at layer 232.
func Encode(s *mtproto.ReactionsNotifySettings) ([]byte, error) {
	if s == nil || s.Sound == nil || s.ShowPreviews == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if s.PredicateName == "" {
		s.PredicateName = mtproto.Predicate_reactionsNotifySettings
	}
	buf := mtproto.NewEncodeBuf(128)
	if err := s.Encode(buf, 232); err != nil {
		return nil, err
	}
	return append([]byte(nil), buf.GetBuf()...), nil
}

// Decode reads a layer 232 reactionsNotifySettings blob.
func Decode(blob []byte) (*mtproto.ReactionsNotifySettings, error) {
	s := &mtproto.ReactionsNotifySettings{}
	if err := s.Decode(mtproto.NewDecodeBuf(blob)); err != nil {
		return nil, err
	}
	return s, nil
}
