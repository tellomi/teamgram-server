package reactions

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/zeromicro/go-zero/core/logx"
)

// Key is the user_settings.key2 row for account.getReactionsNotifySettings.
const Key = "reactions_notify_settings"

// maxValueLen matches user_settings.value (varchar 512).
const maxValueLen = 512

// Store is the per-user reaction notification payload.
type Store interface {
	Load(ctx context.Context, userID int64) (string, bool, error)
	Save(ctx context.Context, userID int64, value string) error
}

// MemoryStore keeps settings in the process. Tests and a missing MySQL
// connection use it so account.getReactionsNotifySettings still succeeds.
type MemoryStore struct {
	mu   sync.Mutex
	rows map[int64]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{rows: map[int64]string{}}
}

func (m *MemoryStore) Load(_ context.Context, userID int64) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.rows[userID]
	return v, ok, nil
}

func (m *MemoryStore) Save(_ context.Context, userID int64, value string) error {
	if len(value) > maxValueLen {
		return errValueTooLong
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[userID] = value
	return nil
}

type mysqlStore struct {
	db *sql.DB
}

// Open uses MySQL when dsn is set and falls back to memory if it is not reachable.
func Open(dsn string) Store {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return NewMemoryStore()
	}
	st, err := newMySQLStore(dsn)
	if err != nil {
		logx.Errorf("reactions notify settings: mysql unavailable, using memory: %v", err)
		return NewMemoryStore()
	}
	return st
}

func newMySQLStore(dsn string) (*mysqlStore, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &mysqlStore{db: db}, nil
}

func (s *mysqlStore) Load(ctx context.Context, userID int64) (string, bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx,
		"select value from user_settings where user_id = ? and key2 = ? and deleted = 0 limit 1",
		userID, Key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (s *mysqlStore) Save(ctx context.Context, userID int64, value string) error {
	if len(value) > maxValueLen {
		return errValueTooLong
	}
	_, err := s.db.ExecContext(ctx,
		"insert into user_settings (user_id, key2, value) values (?, ?, ?) on duplicate key update value = values(value), deleted = 0",
		userID, Key, value)
	return err
}
