package helpdesk

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	settingsKey = "dismissed_suggestions"
	maxValueLen = 512
	maxNameLen  = 64
)

// Suggestions remembers help.dismissSuggestion for each user and peer.
// https://core.telegram.org/method/help.dismissSuggestion
// https://core.telegram.org/api/config#suggestions
type Suggestions struct {
	mu     sync.Mutex
	hidden map[int64]map[string]struct{}
	loaded map[int64]bool
	db     *sql.DB
	offer  []string
}

// NewMemory keeps dismissals in the process.
func NewMemory(offer []string) *Suggestions {
	return &Suggestions{
		hidden: map[int64]map[string]struct{}{},
		loaded: map[int64]bool{},
		offer:  cleanNames(offer),
	}
}

// Open uses MySQL user_settings when dsn is set, otherwise memory.
func Open(dsn string, offer []string) *Suggestions {
	st := NewMemory(offer)
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return st
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		logx.Errorf("helpdesk: mysql unavailable, suggestions stay in memory: %v", err)
		return st
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		logx.Errorf("helpdesk: mysql unavailable, suggestions stay in memory: %v", err)
		return st
	}
	st.db = db
	return st
}

// LoadOffer reads TELLOMI_SUGGESTIONS, a comma-separated list such as
// VALIDATE_PASSWORD,AUTOARCHIVE_POPULAR.
func LoadOffer() []string {
	return cleanNames(strings.Split(os.Getenv("TELLOMI_SUGGESTIONS"), ","))
}

// Dismiss records suggestion for peer. The name must be a short token.
func (s *Suggestions) Dismiss(ctx context.Context, userID int64, peer *mtproto.InputPeer, suggestion string) error {
	if s == nil {
		return mtproto.ErrInputRequestInvalid
	}
	name, err := cleanName(suggestion)
	if err != nil {
		return err
	}
	key, err := peerKey(userID, peer)
	if err != nil {
		return err
	}
	token := key + "|" + name
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load(ctx, userID)
	s.hidden[userID][token] = struct{}{}
	s.save(ctx, userID)
	return nil
}

// Lists splits the configured offers into still-pending and dismissed names
// for peer. Peer nil means the account itself (inputPeerSelf).
func (s *Suggestions) Lists(ctx context.Context, userID int64, peer *mtproto.InputPeer) (pending, dismissed []string) {
	if s == nil {
		return nil, nil
	}
	key, err := peerKey(userID, peer)
	if err != nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load(ctx, userID)
	for _, name := range s.offer {
		token := key + "|" + name
		if _, ok := s.hidden[userID][token]; ok {
			dismissed = append(dismissed, name)
			continue
		}
		pending = append(pending, name)
	}
	return pending, dismissed
}

func (s *Suggestions) load(ctx context.Context, userID int64) {
	if s.loaded[userID] {
		return
	}
	set := map[string]struct{}{}
	if s.db != nil && userID != 0 {
		var value string
		err := s.db.QueryRowContext(ctx,
			"select value from user_settings where user_id = ? and key2 = ? and deleted = 0 limit 1",
			userID, settingsKey).Scan(&value)
		if err != nil && err != sql.ErrNoRows {
			logx.Errorf("helpdesk: load suggestions: %v", err)
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

func (s *Suggestions) save(ctx context.Context, userID int64) {
	if s.db == nil || userID == 0 {
		return
	}
	keys := make([]string, 0, len(s.hidden[userID]))
	for key := range s.hidden[userID] {
		keys = append(keys, key)
	}
	raw, err := json.Marshal(keys)
	if err != nil || len(raw) > maxValueLen {
		logx.Errorf("helpdesk: suggestions for user %d do not fit user_settings", userID)
		return
	}
	_, err = s.db.ExecContext(ctx,
		"insert into user_settings (user_id, key2, value) values (?, ?, ?) on duplicate key update value = values(value), deleted = 0",
		userID, settingsKey, string(raw))
	if err != nil {
		logx.Errorf("helpdesk: save suggestions: %v", err)
	}
}

func cleanNames(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, name := range in {
		name, err := cleanName(name)
		if err != nil || seen[name] || loginEmailSuggestion(name) {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// loginEmailSuggestion is the official nag to attach a login email.
// Email-only accounts already have users.email, and this server does not
// offer that attachment flow. help.getPromoData must not list these names,
// including when TELLOMI_SUGGESTIONS contains them.
func loginEmailSuggestion(name string) bool {
	switch name {
	case "SETUP_LOGIN_EMAIL", "SETUP_LOGIN_EMAIL_NOSKIP":
		return true
	default:
		return false
	}
}

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxNameLen {
		return "", mtproto.ErrInputRequestInvalid
	}
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
		default:
			return "", mtproto.ErrInputRequestInvalid
		}
	}
	return name, nil
}

func peerKey(self int64, peer *mtproto.InputPeer) (string, error) {
	if peer == nil || peer.GetPredicateName() == "" || peer.GetPredicateName() == mtproto.Predicate_inputPeerEmpty || peer.GetPredicateName() == mtproto.Predicate_inputPeerSelf {
		return "1:0", nil
	}
	p := mtproto.FromInputPeer2(self, peer)
	if p == nil {
		return "", mtproto.ErrPeerIdInvalid
	}
	switch p.PeerType {
	case mtproto.PEER_SELF:
		return "1:0", nil
	case mtproto.PEER_USER, mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
		if p.PeerId == 0 {
			return "", mtproto.ErrPeerIdInvalid
		}
		return strings.Join([]string{strconv.FormatInt(int64(p.PeerType), 10), strconv.FormatInt(p.PeerId, 10)}, ":"), nil
	default:
		return "", mtproto.ErrPeerIdInvalid
	}
}
