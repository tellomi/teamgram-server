package discoverrel

import (
	"context"
	"database/sql"
	"strings"

	"github.com/teamgram/teamgram-server/pkg/mysqltls"

	_ "github.com/go-sql-driver/mysql"
)

// MySQL reads user_contacts and discover_subject.
type MySQL struct {
	db *sql.DB
}

// OpenMySQL opens the teamgram database. The DSN is not logged.
func OpenMySQL(dsn string) (*MySQL, error) {
	db, err := mysqltls.Open(dsn)
	if err != nil {
		return nil, err
	}
	return &MySQL{db: db}, nil
}

// Ping checks the connection.
func (m *MySQL) Ping(ctx context.Context) error {
	return m.db.PingContext(ctx)
}

// Save upserts sub → user id. sub is 16 bytes.
func (m *MySQL) Save(ctx context.Context, userID int64, sub []byte) error {
	if userID <= 0 || len(sub) != 16 {
		return errBad
	}
	_, err := m.db.ExecContext(ctx, `
INSERT INTO discover_subject (sub, user_id) VALUES (?, ?)
ON DUPLICATE KEY UPDATE user_id = VALUES(user_id)`, sub, userID)
	return err
}

// Users resolves raw subs. Missing subs are omitted.
func (m *MySQL) Users(ctx context.Context, subs [][]byte) (map[string]int64, error) {
	out := map[string]int64{}
	if len(subs) == 0 {
		return out, nil
	}
	args := make([]any, len(subs))
	for i, sub := range subs {
		args[i] = sub
	}
	rows, err := m.db.QueryContext(ctx, `
SELECT sub, user_id FROM discover_subject WHERE sub IN (`+placeholders(len(subs))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var sub []byte
		var id int64
		if err = rows.Scan(&sub, &id); err != nil {
			return nil, err
		}
		out[subKey(sub)] = id
	}
	return out, rows.Err()
}

// Edges returns authors who currently list viewer in user_contacts
// (is_deleted = 0). close_friend is the Stories flag on that row.
func (m *MySQL) Edges(ctx context.Context, viewer int64, authors []int64) (map[int64]Edge, error) {
	out := map[int64]Edge{}
	if viewer <= 0 || len(authors) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(authors)+1)
	args = append(args, viewer)
	for _, id := range authors {
		args = append(args, id)
	}
	rows, err := m.db.QueryContext(ctx, `
SELECT owner_user_id, close_friend FROM user_contacts
WHERE contact_user_id = ? AND is_deleted = 0 AND owner_user_id IN (`+placeholders(len(authors))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var author int64
		var closeFriend bool
		if err = rows.Scan(&author, &closeFriend); err != nil {
			return nil, err
		}
		out[author] = Edge{Contact: true, CloseFriend: closeFriend}
	}
	return out, rows.Err()
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", n), ",")
}
