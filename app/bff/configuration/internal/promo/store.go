package promo

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
)

// hiddenSettingsKey is the user_settings.key2 row for help.hidePromoData.
const hiddenSettingsKey = "hidden_promo_peers"

// maxValueLen matches user_settings.value (varchar 512).
const maxValueLen = 512

// Store is the per-user set of peers hidden with help.hidePromoData.
type Store struct {
	mu       sync.Mutex
	hidden   map[int64]map[string]struct{}
	loaded   map[int64]bool
	announce *Announcement
	db       *sql.DB
}

// NewMemory keeps hidden peers in the process.
func NewMemory() *Store {
	return &Store{
		hidden: map[int64]map[string]struct{}{},
		loaded: map[int64]bool{},
	}
}

// Open uses MySQL when dsn is set and falls back to memory if it is not reachable.
func Open(dsn string) *Store {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return NewMemory()
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		logx.Errorf("promo: mysql unavailable, using memory: %v", err)
		return NewMemory()
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		logx.Errorf("promo: mysql unavailable, using memory: %v", err)
		return NewMemory()
	}
	st := NewMemory()
	st.db = db
	return st
}

// SetAnnouncement installs the PSA or MTProxy peer help.getPromoData can return.
// Nil clears it. Production does not set one.
func (s *Store) SetAnnouncement(a *Announcement) {
	s.mu.Lock()
	s.announce = a
	s.mu.Unlock()
}

// Get returns help.promoData for a visible announcement, otherwise help.promoDataEmpty.
func (s *Store) Get(ctx context.Context, userID int64) *mtproto.Help_PromoData {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadLocked(ctx, userID)
	if s.announce != nil {
		key := announcementKey(s.announce.Peer)
		if key != "" {
			if _, hidden := s.hidden[userID][key]; !hidden {
				return payload(s.announce)
			}
		}
	}
	return Empty()
}

// Hide records peer so a later announcement for it is omitted from help.getPromoData.
func (s *Store) Hide(ctx context.Context, userID int64, peer *mtproto.InputPeer) error {
	key, err := inputKey(userID, peer)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadLocked(ctx, userID)
	s.hidden[userID][key] = struct{}{}
	s.saveLocked(ctx, userID)
	return nil
}

func (s *Store) loadLocked(ctx context.Context, userID int64) {
	if s.loaded[userID] {
		return
	}
	set := map[string]struct{}{}
	if s.db != nil {
		var value string
		err := s.db.QueryRowContext(ctx,
			"select value from user_settings where user_id = ? and key2 = ? and deleted = 0 limit 1",
			userID, hiddenSettingsKey).Scan(&value)
		if err != nil && err != sql.ErrNoRows {
			logx.Errorf("promo: load hidden peers: %v", err)
		}
		if err == nil && value != "" {
			var keys []string
			if json.Unmarshal([]byte(value), &keys) == nil {
				for _, key := range keys {
					if key != "" {
						set[key] = struct{}{}
					}
				}
			}
		}
	}
	s.hidden[userID] = set
	s.loaded[userID] = true
}

func (s *Store) saveLocked(ctx context.Context, userID int64) {
	if s.db == nil {
		return
	}
	keys := make([]string, 0, len(s.hidden[userID]))
	for key := range s.hidden[userID] {
		keys = append(keys, key)
	}
	raw, err := json.Marshal(keys)
	if err != nil {
		logx.Errorf("promo: encode hidden peers: %v", err)
		return
	}
	if len(raw) > maxValueLen {
		logx.Errorf("promo: hidden peers for user %d exceed %d bytes", userID, maxValueLen)
		return
	}
	_, err = s.db.ExecContext(ctx,
		"insert into user_settings (user_id, key2, value) values (?, ?, ?) on duplicate key update value = values(value), deleted = 0",
		userID, hiddenSettingsKey, string(raw))
	if err != nil {
		logx.Errorf("promo: save hidden peers: %v", err)
	}
}
