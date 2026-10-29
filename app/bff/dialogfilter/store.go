// Package dialogfilter stores chat folders per user.
//
// dialog_filters is keyed by user_id and filter_id. filter_blob is the
// layer 232 DialogFilter the client sent. position is the order from
// messages.updateDialogFiltersOrder. dialog_filter_prefs holds the
// tags_enabled flag returned by messages.getDialogFilters.
//
// teamgramd/deploy/migrate.sh applies
// teamgramd/deploy/sql/migrate-20261029-dialog-filters.sql. The process
// also creates the tables on startup. TEAMGRAM_MYSQL_DSN overrides the DSN.
// An empty DSN, or a database that cannot be reached, keeps the rows in
// memory for this process only.
package dialogfilter

import (
	"context"
	"database/sql"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"

	_ "github.com/go-sql-driver/mysql"
	"github.com/zeromicro/go-zero/core/logx"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS dialog_filters (
  user_id BIGINT NOT NULL,
  filter_id INT NOT NULL,
  position INT NOT NULL,
  filter_blob MEDIUMBLOB NOT NULL,
  PRIMARY KEY (user_id, filter_id),
  KEY dialog_filters_order (user_id, position, filter_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS dialog_filter_prefs (
  user_id BIGINT NOT NULL,
  tags_enabled TINYINT NOT NULL,
  PRIMARY KEY (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

// Folder is one saved dialog filter.
type Folder struct {
	ID       int32
	Position int32
	Filter   *mtproto.DialogFilter
}

// Store is the per-user folder list.
type Store interface {
	List(ctx context.Context, userID int64) (folders []Folder, tagsEnabled bool, err error)
	Put(ctx context.Context, userID int64, id int32, filter *mtproto.DialogFilter) error
	Delete(ctx context.Context, userID int64, id int32) error
	Order(ctx context.Context, userID int64, ids []int32) error
	SetTags(ctx context.Context, userID int64, enabled bool) error
}

type item struct {
	pos  int32
	blob []byte
}

type memoryStore struct {
	mu   sync.Mutex
	rows map[int64]map[int32]*item
	tags map[int64]bool
	seq  map[int64]int32
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		rows: map[int64]map[int32]*item{},
		tags: map[int64]bool{},
		seq:  map[int64]int32{},
	}
}

func (m *memoryStore) List(_ context.Context, userID int64) ([]Folder, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Folder
	for id, it := range m.rows[userID] {
		f, err := DecodeFilter(it.blob)
		if err != nil {
			return nil, false, err
		}
		out = append(out, Folder{ID: id, Position: it.pos, Filter: f})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Position != out[j].Position {
			return out[i].Position < out[j].Position
		}
		return out[i].ID < out[j].ID
	})
	return out, m.tags[userID], nil
}

func (m *memoryStore) Put(_ context.Context, userID int64, id int32, filter *mtproto.DialogFilter) error {
	blob, err := EncodeFilter(filter)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rows[userID] == nil {
		m.rows[userID] = map[int32]*item{}
	}
	if cur := m.rows[userID][id]; cur != nil {
		cur.blob = blob
		return nil
	}
	pos := m.seq[userID]
	m.seq[userID] = pos + 1
	m.rows[userID][id] = &item{pos: pos, blob: blob}
	return nil
}

func (m *memoryStore) Delete(_ context.Context, userID int64, id int32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rows[userID], id)
	return nil
}

func (m *memoryStore) Order(_ context.Context, userID int64, ids []int32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rows := m.rows[userID]
	if len(rows) == 0 {
		return nil
	}
	listed := map[int32]int32{}
	for i, id := range ids {
		listed[id] = int32(i)
	}
	type pair struct {
		id  int32
		pos int32
	}
	var rest []pair
	for id, it := range rows {
		if _, ok := listed[id]; ok {
			it.pos = listed[id]
			continue
		}
		rest = append(rest, pair{id: id, pos: it.pos})
	}
	sort.Slice(rest, func(i, j int) bool {
		if rest[i].pos != rest[j].pos {
			return rest[i].pos < rest[j].pos
		}
		return rest[i].id < rest[j].id
	})
	base := int32(len(ids))
	for i, p := range rest {
		rows[p.id].pos = base + int32(i)
	}
	return nil
}

func (m *memoryStore) SetTags(_ context.Context, userID int64, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tags[userID] = enabled
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
		logx.Errorf("dialogfilter: mysql unavailable, folders stay in memory until restart: %v", err)
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
	for _, stmt := range strings.Split(schemaSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err = db.ExecContext(ctx, stmt); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return &mysqlStore{db: db}, nil
}

func (s *mysqlStore) List(ctx context.Context, userID int64) ([]Folder, bool, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT filter_id, position, filter_blob FROM dialog_filters WHERE user_id=? ORDER BY position, filter_id`,
		userID)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []Folder
	for rows.Next() {
		var id, pos int32
		var blob []byte
		if err = rows.Scan(&id, &pos, &blob); err != nil {
			return nil, false, err
		}
		f, err := DecodeFilter(blob)
		if err != nil {
			return nil, false, err
		}
		out = append(out, Folder{ID: id, Position: pos, Filter: f})
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	var tags int
	err = s.db.QueryRowContext(ctx,
		`SELECT tags_enabled FROM dialog_filter_prefs WHERE user_id=?`, userID).Scan(&tags)
	if err == sql.ErrNoRows {
		return out, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return out, tags != 0, nil
}

func (s *mysqlStore) Put(ctx context.Context, userID int64, id int32, filter *mtproto.DialogFilter) error {
	blob, err := EncodeFilter(filter)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var existing int
	err = tx.QueryRowContext(ctx,
		`SELECT 1 FROM dialog_filters WHERE user_id=? AND filter_id=?`, userID, id).Scan(&existing)
	if err == sql.ErrNoRows {
		var maxPos sql.NullInt32
		if err = tx.QueryRowContext(ctx,
			`SELECT MAX(position) FROM dialog_filters WHERE user_id=?`, userID).Scan(&maxPos); err != nil {
			return err
		}
		pos := int32(0)
		if maxPos.Valid {
			pos = maxPos.Int32 + 1
		}
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO dialog_filters (user_id, filter_id, position, filter_blob) VALUES (?, ?, ?, ?)`,
			userID, id, pos, blob); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if _, err = tx.ExecContext(ctx,
		`UPDATE dialog_filters SET filter_blob=? WHERE user_id=? AND filter_id=?`,
		blob, userID, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *mysqlStore) Delete(ctx context.Context, userID int64, id int32) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM dialog_filters WHERE user_id=? AND filter_id=?`, userID, id)
	return err
}

func (s *mysqlStore) Order(ctx context.Context, userID int64, ids []int32) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx,
		`SELECT filter_id, position FROM dialog_filters WHERE user_id=? ORDER BY position, filter_id`, userID)
	if err != nil {
		return err
	}
	type pair struct {
		id  int32
		pos int32
	}
	var have []pair
	for rows.Next() {
		var p pair
		if err = rows.Scan(&p.id, &p.pos); err != nil {
			_ = rows.Close()
			return err
		}
		have = append(have, p)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	listed := map[int32]int32{}
	for i, id := range ids {
		listed[id] = int32(i)
	}
	base := int32(len(ids))
	extra := int32(0)
	for _, p := range have {
		pos, ok := listed[p.id]
		if !ok {
			pos = base + extra
			extra++
		}
		if _, err = tx.ExecContext(ctx,
			`UPDATE dialog_filters SET position=? WHERE user_id=? AND filter_id=?`,
			pos, userID, p.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *mysqlStore) SetTags(ctx context.Context, userID int64, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO dialog_filter_prefs (user_id, tags_enabled) VALUES (?, ?)
		 ON DUPLICATE KEY UPDATE tags_enabled=VALUES(tags_enabled)`,
		userID, v)
	return err
}

var (
	mu      sync.RWMutex
	current Store = newMemoryStore()
)

// Open stores the process-wide folder list. An empty dsn uses memory unless
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

// EncodeFilter serializes a DialogFilter at layer 232.
func EncodeFilter(f *mtproto.DialogFilter) ([]byte, error) {
	if f == nil || f.GetPredicateName() == "" {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if f.GetPredicateName() != mtproto.Predicate_dialogFilterDefault && f.Title_TEXTWITHENTITIES == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	buf := mtproto.NewEncodeBuf(256)
	if err := f.Encode(buf, 232); err != nil {
		return nil, err
	}
	return append([]byte(nil), buf.GetBuf()...), nil
}

// DecodeFilter reads a layer 232 DialogFilter blob.
func DecodeFilter(blob []byte) (*mtproto.DialogFilter, error) {
	f := &mtproto.DialogFilter{}
	if err := f.Decode(mtproto.NewDecodeBuf(blob)); err != nil {
		return nil, err
	}
	return f, nil
}
