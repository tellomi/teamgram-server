package terms

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/zeromicro/go-zero/core/logx"
)

// Store remembers which terms id a user has accepted.
type Store interface {
	Accepted(ctx context.Context, userID int64, tosID string) (bool, error)
	Accept(ctx context.Context, userID int64, tosID string) error
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS user_tos_acceptances (
  user_id BIGINT NOT NULL,
  tos_id VARCHAR(128) NOT NULL,
  accepted_at INT NOT NULL,
  PRIMARY KEY (user_id, tos_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

type memoryStore struct {
	mu   sync.Mutex
	rows map[int64]map[string]int64
}

func newMemoryStore() *memoryStore {
	return &memoryStore{rows: map[int64]map[string]int64{}}
}

func (m *memoryStore) Accepted(_ context.Context, userID int64, tosID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.rows[userID][tosID]
	return ok, nil
}

func (m *memoryStore) Accept(_ context.Context, userID int64, tosID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rows[userID] == nil {
		m.rows[userID] = map[string]int64{}
	}
	if _, ok := m.rows[userID][tosID]; !ok {
		m.rows[userID][tosID] = time.Now().Unix()
	}
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
		logx.Errorf("tos: mysql unavailable, acceptance stays in memory until restart: %v", err)
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
	if _, err = db.ExecContext(ctx, schemaSQL); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &mysqlStore{db: db}, nil
}

func (s *mysqlStore) Accepted(ctx context.Context, userID int64, tosID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM user_tos_acceptances WHERE user_id=? AND tos_id=? LIMIT 1`,
		userID, tosID).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *mysqlStore) Accept(ctx context.Context, userID int64, tosID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_tos_acceptances (user_id, tos_id, accepted_at) VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE accepted_at=accepted_at`,
		userID, tosID, time.Now().Unix())
	return err
}
