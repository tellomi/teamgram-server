package strippedbackfill

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/go-sql-driver/mysql"
)

// MySQL reads profile photo ids and writes the size "i" row.
type MySQL struct {
	db         *sql.DB
	noChannels bool
}

func NewMySQL(db *sql.DB) *MySQL {
	return &MySQL{db: db}
}

func (m *MySQL) Next(ctx context.Context, after int64, limit int) ([]int64, error) {
	if limit <= 0 {
		return nil, nil
	}
	sets := [][]int64{}
	queries := []struct {
		q    string
		skip bool
	}{
		{q: `SELECT photo_id FROM users WHERE photo_id > ? ORDER BY photo_id LIMIT ?`},
		{q: `SELECT photo_id FROM user_profile_photos WHERE deleted = 0 AND photo_id > ? ORDER BY photo_id LIMIT ?`},
		{q: `SELECT photo_id FROM chats WHERE photo_id > ? ORDER BY photo_id LIMIT ?`},
		{q: `SELECT photo_id FROM channels WHERE photo_id > ? ORDER BY photo_id LIMIT ?`, skip: m.noChannels},
	}
	for i, item := range queries {
		if item.skip {
			continue
		}
		ids, err := m.ids(ctx, item.q, after, limit)
		if err != nil {
			if i == 3 && missingTable(err) {
				m.noChannels = true
				continue
			}
			return nil, err
		}
		sets = append(sets, ids)
	}
	return mergeIDs(sets, limit), nil
}

func (m *MySQL) ids(ctx context.Context, q string, after int64, limit int) ([]int64, error) {
	rows, err := m.db.QueryContext(ctx, q, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if id > 0 {
			out = append(out, id)
		}
	}
	return out, rows.Err()
}

func mergeIDs(sets [][]int64, limit int) []int64 {
	seen := map[int64]struct{}{}
	var ids []int64
	for _, set := range sets {
		for _, id := range set {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids
}

func (m *MySQL) Stripped(ctx context.Context, photoID int64) (exists, filled bool, err error) {
	var cached sql.NullString
	err = m.db.QueryRowContext(ctx, `
SELECT cached_bytes FROM photo_sizes WHERE photo_size_id = ? AND size_type = 'i' LIMIT 1`, photoID).Scan(&cached)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return true, cached.Valid && cached.String != "", nil
}

func (m *MySQL) Paths(ctx context.Context, photoID int64) (map[string]string, error) {
	out := map[string]string{}
	if err := m.collectPaths(ctx, out, `
SELECT size_type, file_path FROM photo_sizes WHERE photo_size_id = ?`, photoID); err != nil {
		return nil, err
	}
	if err := m.collectPaths(ctx, out, `
SELECT size_type, file_path FROM media_photo_paths WHERE photo_id = ?`, photoID); err != nil && !missingTable(err) {
		return nil, err
	}
	return out, nil
}

func (m *MySQL) collectPaths(ctx context.Context, out map[string]string, q string, photoID int64) error {
	rows, err := m.db.QueryContext(ctx, q, photoID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var sz, path string
		if err := rows.Scan(&sz, &path); err != nil {
			return err
		}
		if sz == "" || sz == "i" || sz == "j" {
			continue
		}
		if path != "" {
			out[sz] = path
		} else if _, ok := out[sz]; !ok {
			out[sz] = ""
		}
	}
	return rows.Err()
}

func (m *MySQL) Owners(ctx context.Context, photoID int64) (users, chats []int64, err error) {
	users, err = m.oneCol(ctx, `SELECT id FROM users WHERE photo_id = ?`, photoID)
	if err != nil {
		return nil, nil, err
	}
	album, err := m.oneCol(ctx, `SELECT user_id FROM user_profile_photos WHERE photo_id = ? AND deleted = 0`, photoID)
	if err != nil {
		return nil, nil, err
	}
	users = append(users, album...)
	chats, err = m.oneCol(ctx, `SELECT id FROM chats WHERE photo_id = ?`, photoID)
	if err != nil {
		return nil, nil, err
	}
	return users, chats, nil
}

func (m *MySQL) oneCol(ctx context.Context, q string, arg int64) ([]int64, error) {
	rows, err := m.db.QueryContext(ctx, q, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (m *MySQL) SaveStripped(ctx context.Context, photoID int64, raw []byte, cached string) error {
	path := fmt.Sprintf("i/%d.dat", photoID)
	_, err := m.db.ExecContext(ctx, `
INSERT INTO photo_sizes (photo_size_id, size_type, width, height, file_size, file_path, cached_type, cached_bytes)
SELECT ?, 'i', 0, 0, ?, ?, 2, ? FROM DUAL
WHERE NOT EXISTS (
  SELECT 1 FROM photo_sizes WHERE photo_size_id = ? AND size_type = 'i'
)`, photoID, len(raw), path, cached, photoID)
	if err != nil && !dup(err) {
		return err
	}
	_, err = m.db.ExecContext(ctx, `
UPDATE photo_sizes
SET file_size = ?, file_path = ?, cached_type = 2, cached_bytes = ?
WHERE photo_size_id = ? AND size_type = 'i' AND cached_bytes = ''`,
		len(raw), path, cached, photoID)
	return err
}

func missingTable(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1146
}

func dup(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}
