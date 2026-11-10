package discoverpush

import (
	"context"
	"database/sql"
	"strings"

	"github.com/teamgram/teamgram-server/pkg/mysqltls"

	_ "github.com/go-sql-driver/mysql"
)

// MySQLEvents claims rows in discover_push_event.
type MySQLEvents struct {
	db *sql.DB
}

// OpenEvents opens the teamgram database. The DSN is not logged.
func OpenEvents(dsn string) (*MySQLEvents, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, errBad
	}
	db, err := mysqltls.Open(dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(1)
	return &MySQLEvents{db: db}, nil
}

// Claim inserts the event id. A duplicate primary key is not fresh.
func (m *MySQLEvents) Claim(ctx context.Context, id string, userID int64) (bool, error) {
	res, err := m.db.ExecContext(ctx, `
INSERT IGNORE INTO discover_push_event (event_id, user_id) VALUES (?, ?)`, id, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// Release deletes the claim after a push that could not be attempted.
func (m *MySQLEvents) Release(ctx context.Context, id string) error {
	_, err := m.db.ExecContext(ctx, `DELETE FROM discover_push_event WHERE event_id = ?`, id)
	return err
}
