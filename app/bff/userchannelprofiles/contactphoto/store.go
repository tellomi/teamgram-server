package contactphoto

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/mysqlpool"
	"github.com/teamgram/teamgram-server/pkg/mysqltls"
	"google.golang.org/protobuf/proto"
)

// schemaSQL is the personal-photo table. The same statement is
// teamgramd/deploy/sql/migrate-20261129-contact-profile-photo.sql.
// Creating it again does not change rows that are already there.
const schemaSQL = `
CREATE TABLE IF NOT EXISTS ` + "`user_contact_photos`" + ` (
  ` + "`owner_user_id`" + ` bigint NOT NULL,
  ` + "`contact_user_id`" + ` bigint NOT NULL,
  ` + "`photo_id`" + ` bigint NOT NULL,
  ` + "`photo`" + ` mediumblob NOT NULL,
  ` + "`created_at`" + ` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  ` + "`updated_at`" + ` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (` + "`owner_user_id`" + `, ` + "`contact_user_id`" + `)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
`

type book interface {
	put(ctx context.Context, owner, contact int64, photo *mtproto.Photo) error
	get(ctx context.Context, owner, contact int64) (*mtproto.Photo, bool, error)
	del(ctx context.Context, owner, contact int64) error
}

type memBook struct {
	mu    sync.Mutex
	items map[key]*mtproto.Photo
}

func newMem() *memBook {
	return &memBook{items: map[key]*mtproto.Photo{}}
}

func (m *memBook) put(_ context.Context, owner, contact int64, photo *mtproto.Photo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[key{owner, contact, false}] = proto.Clone(photo).(*mtproto.Photo)
	return nil
}

func (m *memBook) get(_ context.Context, owner, contact int64) (*mtproto.Photo, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	photo, ok := m.items[key{owner, contact, false}]
	if !ok || photo == nil {
		return nil, false, nil
	}
	return proto.Clone(photo).(*mtproto.Photo), true, nil
}

func (m *memBook) del(_ context.Context, owner, contact int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, key{owner, contact, false})
	return nil
}

type sqlBook struct{ db *sql.DB }

func (s sqlBook) put(ctx context.Context, owner, contact int64, photo *mtproto.Photo) error {
	raw, err := proto.Marshal(photo)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO user_contact_photos (owner_user_id, contact_user_id, photo_id, photo)
VALUES (?,?,?,?)
ON DUPLICATE KEY UPDATE photo_id=VALUES(photo_id), photo=VALUES(photo)`,
		owner, contact, photo.GetId(), raw)
	return err
}

func (s sqlBook) get(ctx context.Context, owner, contact int64) (*mtproto.Photo, bool, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `
SELECT photo FROM user_contact_photos WHERE owner_user_id=? AND contact_user_id=?`,
		owner, contact).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	photo := &mtproto.Photo{}
	if err = proto.Unmarshal(raw, photo); err != nil {
		return nil, false, err
	}
	return photo, true, nil
}

func (s sqlBook) del(ctx context.Context, owner, contact int64) error {
	_, err := s.db.ExecContext(ctx, `
DELETE FROM user_contact_photos WHERE owner_user_id=? AND contact_user_id=?`, owner, contact)
	return err
}

var (
	bookMu sync.Mutex
	saved  book = newMem()
	// suggested is the photo a suggest call offered. The service message is
	// what the contact receives. This map lets tests see the offer when no
	// messenger is attached.
	suggestMu sync.Mutex
	suggested = map[key]*mtproto.Photo{}
)

func active() book {
	bookMu.Lock()
	defer bookMu.Unlock()
	return saved
}

// UseDSN opens the shared personal-photo table when dsn or
// TEAMGRAM_MYSQL_DSN is set. An empty DSN keeps the process-local book.
func UseDSN(dsn string) error {
	if env := os.Getenv("TEAMGRAM_MYSQL_DSN"); env != "" {
		dsn = env
	}
	if dsn == "" {
		return nil
	}
	db, err := mysqltls.Open(dsn)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = mysqlpool.Discard(db)
		return err
	}
	if _, err = db.ExecContext(ctx, schemaSQL); err != nil {
		_ = mysqlpool.Discard(db)
		return err
	}
	bookMu.Lock()
	saved = sqlBook{db: db}
	bookMu.Unlock()
	return nil
}

// Reset drops process-local photos. Tests call it. A MySQL book is left in
// place so a test cannot wipe the table.
func Reset() {
	bookMu.Lock()
	if _, ok := saved.(*memBook); ok {
		saved = newMem()
	}
	bookMu.Unlock()
	suggestMu.Lock()
	suggested = map[key]*mtproto.Photo{}
	suggestMu.Unlock()
}
