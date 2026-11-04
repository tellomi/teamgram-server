package announce

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// userPageSQL lists live accounts after a cursor. Official accounts
// 777000–777005 cannot sign in. user_type 4 is the service account.
// This statement does not touch Redis.
const userPageSQL = `
SELECT id FROM users
WHERE deleted = 0 AND is_bot = 0 AND user_type <> 4 AND id > ?
  AND id NOT BETWEEN 777000 AND 777005
ORDER BY id
LIMIT %d`

// MySQL stores claims in tellomi_announcement.
type MySQL struct {
	db *sql.DB
}

// OpenMySQL opens the teamgram database. The DSN is not logged.
func OpenMySQL(dsn string) (*MySQL, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, errBad
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(1)
	return &MySQL{db: db}, nil
}

// Page implements Directory.
func (m *MySQL) Page(ctx context.Context, after int64, limit int) ([]int64, error) {
	if m == nil || m.db == nil {
		return nil, ErrUnavailable
	}
	if limit < 1 || limit > PageSize {
		limit = PageSize
	}
	rows, err := m.db.QueryContext(ctx, fmt.Sprintf(userPageSQL, limit), after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ClaimUser implements Claims.
func (m *MySQL) ClaimUser(ctx context.Context, eventID string, userID int64) (bool, error) {
	return m.insert(ctx, `INSERT INTO tellomi_announcement (event_id, user_id) VALUES (?, ?)`, eventID, userID)
}

// ReleaseUser implements Claims.
func (m *MySQL) ReleaseUser(ctx context.Context, eventID string, userID int64) error {
	if m == nil || m.db == nil {
		return ErrUnavailable
	}
	_, err := m.db.ExecContext(ctx, `DELETE FROM tellomi_announcement WHERE event_id = ? AND user_id = ?`, eventID, userID)
	return err
}

// ClaimChannel implements Claims.
func (m *MySQL) ClaimChannel(ctx context.Context, eventID string) (bool, error) {
	return m.insert(ctx, `INSERT INTO tellomi_announcement_channel (event_id) VALUES (?)`, eventID)
}

// ReleaseChannel implements Claims.
func (m *MySQL) ReleaseChannel(ctx context.Context, eventID string) error {
	if m == nil || m.db == nil {
		return ErrUnavailable
	}
	_, err := m.db.ExecContext(ctx, `DELETE FROM tellomi_announcement_channel WHERE event_id = ?`, eventID)
	return err
}

func (m *MySQL) insert(ctx context.Context, query string, args ...any) (bool, error) {
	if m == nil || m.db == nil {
		return false, ErrUnavailable
	}
	_, err := m.db.ExecContext(ctx, query, args...)
	if err == nil {
		return true, nil
	}
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1062 {
		return false, nil
	}
	return false, err
}
