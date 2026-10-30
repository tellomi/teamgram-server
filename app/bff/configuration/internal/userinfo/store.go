package userinfo

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	_ "github.com/go-sql-driver/mysql"
	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	maxMessage  = 4096
	maxEntities = 100
)

// Store is the support note help.getUserInfo and help.editUserInfo share.
type Store struct {
	mu    sync.Mutex
	notes map[int64]note
	db    *sql.DB
}

type note struct {
	Message  string
	Entities []*mtproto.MessageEntity
	Author   string
	Date     int32
}

// NewMemory keeps notes in the process.
func NewMemory() *Store {
	return &Store{notes: map[int64]note{}}
}

// Open uses MySQL when dsn is set and falls back to memory if it is not reachable.
func Open(dsn string) *Store {
	st := NewMemory()
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return st
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		logx.Errorf("userinfo: mysql unavailable, using memory: %v", err)
		return st
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		logx.Errorf("userinfo: mysql unavailable, using memory: %v", err)
		return st
	}
	if _, err = db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS tellomi_user_info (
  user_id BIGINT NOT NULL,
  message VARCHAR(4096) NOT NULL,
  entities_json MEDIUMTEXT NOT NULL,
  author VARCHAR(64) NOT NULL,
  date_ts INT NOT NULL,
  PRIMARY KEY (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		_ = db.Close()
		logx.Errorf("userinfo: schema: %v", err)
		return st
	}
	st.db = db
	return st
}

// Get returns the note for userID, or help.userInfoEmpty when none is stored.
func (s *Store) Get(ctx context.Context, userID int64) (*mtproto.Help_UserInfo, error) {
	if userID <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load(ctx, userID)
	n, ok := s.notes[userID]
	if !ok || strings.TrimSpace(n.Message) == "" {
		return mtproto.MakeTLHelpUserInfoEmpty(&mtproto.Help_UserInfo{}).To_Help_UserInfo(), nil
	}
	return n.tl(), nil
}

// Edit stores a note. An empty message clears it and returns help.userInfoEmpty.
func (s *Store) Edit(ctx context.Context, editor, userID int64, message string, entities []*mtproto.MessageEntity) (*mtproto.Help_UserInfo, error) {
	if userID <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	message = strings.TrimSpace(message)
	if utf8.RuneCountInString(message) > maxMessage {
		return nil, mtproto.ErrMessageTooLong
	}
	if len(entities) > maxEntities {
		return nil, mtproto.ErrEntitiesTooLong
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if message == "" {
		delete(s.notes, userID)
		if err := s.delete(ctx, userID); err != nil {
			return nil, err
		}
		return mtproto.MakeTLHelpUserInfoEmpty(&mtproto.Help_UserInfo{}).To_Help_UserInfo(), nil
	}
	if entities == nil {
		entities = []*mtproto.MessageEntity{}
	}
	n := note{
		Message:  message,
		Entities: append([]*mtproto.MessageEntity(nil), entities...),
		Author:   strconv.FormatInt(editor, 10),
		Date:     int32(time.Now().Unix()),
	}
	if err := s.save(ctx, userID, n); err != nil {
		return nil, err
	}
	s.notes[userID] = n
	return n.tl(), nil
}

func (n note) tl() *mtproto.Help_UserInfo {
	entities := n.Entities
	if entities == nil {
		entities = []*mtproto.MessageEntity{}
	}
	return mtproto.MakeTLHelpUserInfo(&mtproto.Help_UserInfo{
		Message:  n.Message,
		Entities: entities,
		Author:   n.Author,
		Date:     n.Date,
	}).To_Help_UserInfo()
}

func (s *Store) load(ctx context.Context, userID int64) {
	if s.db == nil {
		return
	}
	if _, ok := s.notes[userID]; ok {
		return
	}
	var message, raw, author string
	var date int32
	err := s.db.QueryRowContext(ctx, `SELECT message, entities_json, author, date_ts FROM tellomi_user_info WHERE user_id=?`, userID).Scan(&message, &raw, &author, &date)
	if err != nil {
		return
	}
	var entities []*mtproto.MessageEntity
	if raw != "" && raw != "null" {
		_ = json.Unmarshal([]byte(raw), &entities)
	}
	s.notes[userID] = note{Message: message, Entities: entities, Author: author, Date: date}
}

func (s *Store) save(ctx context.Context, userID int64, n note) error {
	if s.db == nil {
		return nil
	}
	raw, err := json.Marshal(n.Entities)
	if err != nil {
		raw = []byte("[]")
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO tellomi_user_info (user_id, message, entities_json, author, date_ts) VALUES (?,?,?,?,?)
ON DUPLICATE KEY UPDATE message=VALUES(message), entities_json=VALUES(entities_json), author=VALUES(author), date_ts=VALUES(date_ts)`,
		userID, n.Message, string(raw), n.Author, n.Date)
	return err
}

func (s *Store) delete(ctx context.Context, userID int64) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM tellomi_user_info WHERE user_id=?`, userID)
	return err
}
