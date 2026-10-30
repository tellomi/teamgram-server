package chatbox

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/teamgram/proto/mtproto"
)

// docDB is the optional MySQL backing for chatbox aggregates. BFF and the
// msg service share one table when TEAMGRAM_MYSQL_DSN (or the channel DSN)
// is set, so a scheduled message stored in one process is visible to the other.
type docDB interface {
	Load(ctx context.Context, key string) ([]byte, bool, error)
	Save(ctx context.Context, key string, raw []byte) error
}

type sqlDocs struct{ db *sql.DB }

func (s sqlDocs) Load(ctx context.Context, key string) ([]byte, bool, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT data FROM chatbox_doc WHERE doc_key=?`, key).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

func (s sqlDocs) Save(ctx context.Context, key string, raw []byte) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO chatbox_doc (doc_key, data) VALUES (?,?)
ON DUPLICATE KEY UPDATE data=VALUES(data)`, key, raw)
	return err
}

const docSchema = `
CREATE TABLE IF NOT EXISTS chatbox_doc (
  doc_key VARCHAR(191) NOT NULL PRIMARY KEY,
  data MEDIUMBLOB NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
`

// Book is the process-wide store for polls, scheduled messages, private-chat
// reactions, the default double-tap reaction, in-chat bot state (inline
// results, attach menu, web apps, prepared messages), quick replies,
// translation flags, saved-message reaction tags, and message reports.
type Book struct {
	mu        sync.Mutex
	db        docDB
	sched     map[string]*schedDoc
	polls     map[int64]*pollDoc
	pollIdx   map[string]int64
	pollPeer  map[string][]int64
	react     map[string]*reactDoc
	def       map[int64]string
	bots      map[int64]*botState
	queries   map[int64]*inlineQuery
	attach    map[int64]*attachDoc
	prepared  map[string]*preparedDoc
	webviews  map[int64]*webSession
	callbacks map[string]*callbackAnswer
	cbBind    map[int64]*callbackBind
	sent      map[string][]byte
	quick     map[int64]*quickDoc
	plain     map[string]*plainDoc
	transOff  map[string]bool
	tags      map[int64]*tagDoc
	paid      map[string][]*mtproto.MessageExtendedMedia
	reports   []reportItem
	now       func() time.Time
}

func newBook(db docDB) *Book {
	return &Book{
		db:        db,
		sched:     map[string]*schedDoc{},
		polls:     map[int64]*pollDoc{},
		pollIdx:   map[string]int64{},
		pollPeer:  map[string][]int64{},
		react:     map[string]*reactDoc{},
		def:       map[int64]string{},
		bots:      map[int64]*botState{},
		queries:   map[int64]*inlineQuery{},
		attach:    map[int64]*attachDoc{},
		prepared:  map[string]*preparedDoc{},
		webviews:  map[int64]*webSession{},
		callbacks: map[string]*callbackAnswer{},
		cbBind:    map[int64]*callbackBind{},
		sent:      map[string][]byte{},
		quick:     map[int64]*quickDoc{},
		plain:     map[string]*plainDoc{},
		transOff:  map[string]bool{},
		tags:      map[int64]*tagDoc{},
		paid:      map[string][]*mtproto.MessageExtendedMedia{},
		now:       time.Now,
	}
}

var (
	regMu  sync.Mutex
	shared = newBook(nil)
)

// Shared is the process-wide book.
func Shared() *Book {
	regMu.Lock()
	defer regMu.Unlock()
	return shared
}

// ResetShared drops process state. Tests call it so cases do not leak.
func ResetShared() {
	regMu.Lock()
	shared = newBook(nil)
	regMu.Unlock()
}

// UseDSN opens MySQL when dsn or TEAMGRAM_MYSQL_DSN is set.
func UseDSN(dsn string) error {
	if env := os.Getenv("TEAMGRAM_MYSQL_DSN"); env != "" {
		dsn = env
	}
	if dsn == "" {
		return nil
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return err
	}
	if _, err = db.ExecContext(ctx, docSchema); err != nil {
		_ = db.Close()
		return err
	}
	regMu.Lock()
	prev := shared
	shared = newBook(sqlDocs{db: db})
	shared.now = prev.now
	regMu.Unlock()
	return nil
}
