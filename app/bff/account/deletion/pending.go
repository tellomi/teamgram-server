package deletion

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/teamgram/teamgram-server/pkg/mysqlpool"
	"github.com/teamgram/teamgram-server/pkg/mysqltls"

	_ "github.com/go-sql-driver/mysql"
	"github.com/zeromicro/go-zero/core/logx"
)

// Item is one account waiting out the seven-day deletion.
type Item struct {
	UserID   int64
	DeleteAt int64
	Reason   string
}

// Store remembers scheduled deletions. Memory is the default.
type Store interface {
	Put(ctx context.Context, item Item) error
	Delete(ctx context.Context, userID int64) error
	Due(ctx context.Context, now int64, limit int) ([]Item, error)
}

var (
	mu      sync.RWMutex
	current Store = newMemory()
)

// Use swaps the process-wide store. Tests pass a fresh memory store.
func Use(st Store) Store {
	mu.Lock()
	prev := current
	if st == nil {
		st = newMemory()
	}
	current = st
	mu.Unlock()
	return prev
}

func store() Store {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Schedule records a deletion at deleteAt. A later call replaces the row.
func Schedule(ctx context.Context, userID, deleteAt int64, reason string) error {
	if userID == 0 {
		return nil
	}
	if len(reason) > 128 {
		reason = reason[:128]
	}
	return store().Put(ctx, Item{UserID: userID, DeleteAt: deleteAt, Reason: reason})
}

// Cancel drops a scheduled deletion. Login calls this.
func Cancel(ctx context.Context, userID int64) error {
	if userID == 0 {
		return nil
	}
	return store().Delete(ctx, userID)
}

// ConfigureMySQL points the store at teamgram. An empty DSN keeps memory.
// The table is created if the migration has not run.
func ConfigureMySQL(dsn string) {
	if dsn == "" {
		return
	}
	db, err := mysqltls.Open(dsn)
	if err != nil {
		logx.Errorf("account deletion: mysql open failed, schedule stays in memory: %v", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		logx.Errorf("account deletion: mysql ping failed, schedule stays in memory: %v", err)
		_ = mysqlpool.Discard(db)
		return
	}
	if _, err = db.ExecContext(ctx, pendingSchema); err != nil {
		logx.Errorf("account deletion: schema: %v", err)
	}
	Use(&mysqlStore{db: db})
}

const pendingSchema = `
CREATE TABLE IF NOT EXISTS account_deletion_pending (
  user_id BIGINT NOT NULL,
  delete_at BIGINT NOT NULL,
  reason VARCHAR(128) NOT NULL DEFAULT '',
  created_at BIGINT NOT NULL,
  PRIMARY KEY (user_id),
  KEY delete_at (delete_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
`

type memoryStore struct {
	mu    sync.Mutex
	items map[int64]Item
}

func newMemory() *memoryStore {
	return &memoryStore{items: map[int64]Item{}}
}

func (m *memoryStore) Put(_ context.Context, item Item) error {
	m.mu.Lock()
	m.items[item.UserID] = item
	m.mu.Unlock()
	return nil
}

func (m *memoryStore) Delete(_ context.Context, userID int64) error {
	m.mu.Lock()
	delete(m.items, userID)
	m.mu.Unlock()
	return nil
}

func (m *memoryStore) Due(_ context.Context, now int64, limit int) ([]Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 {
		limit = 50
	}
	var out []Item
	for _, it := range m.items {
		if it.DeleteAt <= now {
			out = append(out, it)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

type mysqlStore struct {
	db *sql.DB
}

func (s *mysqlStore) Put(ctx context.Context, item Item) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO account_deletion_pending (user_id, delete_at, reason, created_at)
VALUES (?,?,?,?)
ON DUPLICATE KEY UPDATE delete_at = VALUES(delete_at), reason = VALUES(reason)`,
		item.UserID, item.DeleteAt, item.Reason, time.Now().Unix())
	return err
}

func (s *mysqlStore) Delete(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM account_deletion_pending WHERE user_id = ?`, userID)
	return err
}

func (s *mysqlStore) Due(ctx context.Context, now int64, limit int) ([]Item, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT user_id, delete_at, reason FROM account_deletion_pending
WHERE delete_at <= ? ORDER BY delete_at LIMIT ?`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var it Item
		if err = rows.Scan(&it.UserID, &it.DeleteAt, &it.Reason); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// RunDue deletes accounts whose wait has elapsed. purge must be the same
// cleanup as account.deleteAccount. A purge error leaves the row for the
// next pass.
func RunDue(ctx context.Context, now time.Time, purge func(context.Context, int64, string) error) error {
	due, err := store().Due(ctx, now.Unix(), 50)
	if err != nil {
		return err
	}
	for _, it := range due {
		if err = purge(ctx, it.UserID, it.Reason); err != nil {
			logx.WithContext(ctx).Errorf("account deletion: user %d: %v", it.UserID, err)
			continue
		}
		_ = store().Delete(ctx, it.UserID)
	}
	return nil
}

// StartReaper runs RunDue until ctx is cancelled.
func StartReaper(ctx context.Context, every time.Duration, purge func(context.Context, int64, string) error) {
	if every <= 0 {
		every = time.Minute
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				if err := RunDue(ctx, now, purge); err != nil {
					logx.Errorf("account deletion reaper: %v", err)
				}
			}
		}
	}()
}
