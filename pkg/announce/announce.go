package announce

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	// EnvEnabled turns the listener on. Only the value 1 counts.
	EnvEnabled = "TELLOMI_ANNOUNCE"
	// EnvKey is standard base64 of at least 32 bytes. The caller sends that
	// same base64 as Authorization: Bearer.
	EnvKey = "TELLOMI_ANNOUNCE_KEY"
	// EnvAddr is the listen address. Empty means 127.0.0.1:8815. "off" does not listen.
	EnvAddr = "TELLOMI_ANNOUNCE_ADDR"

	defaultAddr = "127.0.0.1:8815"
	maxBody     = 16 << 10
	pathPost    = "/internal/v1/announce"

	// ServiceUserID is @admin, the account that already sends login codes.
	ServiceUserID int64 = 777000
	// NewsChannelID is @news (官方公告). The post is the archive copy.
	NewsChannelID int64 = 1073800001
	// NewsPosterID is @tellomi_official, the channel creator.
	NewsPosterID int64 = 777002

	// PageSize is how many private copies one request writes.
	PageSize = 50
	// MaxTextRunes is the longest announcement.
	MaxTextRunes = 1000
)

// ErrUnavailable means the caller can retry the same cursor.
var ErrUnavailable = errors.New("announce: unavailable")

var errBad = errors.New("announce: bad request")

// Users writes one private copy into the @admin chat.
type Users interface {
	Send(ctx context.Context, userID int64, text string) error
}

// Channels writes the single @news post for an event.
type Channels interface {
	Post(ctx context.Context, text string) error
}

// Directory pages live accounts. Official ids 777000–777005 are omitted.
type Directory interface {
	Page(ctx context.Context, after int64, limit int) ([]int64, error)
}

// Claims remembers who already received an event.
type Claims interface {
	ClaimUser(ctx context.Context, eventID string, userID int64) (fresh bool, err error)
	ReleaseUser(ctx context.Context, eventID string, userID int64) error
	ClaimChannel(ctx context.Context, eventID string) (fresh bool, err error)
	ReleaseChannel(ctx context.Context, eventID string) error
}

// Service is one page of a broadcast.
type Service struct {
	Dir      Directory
	Claims   Claims
	Users    Users
	Channel  Channels
	PageSize int
}

// Request is one page. Cursor is the last user id already passed, or 0.
type Request struct {
	EventID string
	Text    string
	After   int64
}

// Result is what the caller uses to continue.
// Channel is "posted", "failed", or "skipped".
type Result struct {
	Status  string
	Cursor  int64
	Sent    int
	Channel string
}

func (s *Service) limit() int {
	if s == nil || s.PageSize < 1 || s.PageSize > PageSize {
		return PageSize
	}
	return s.PageSize
}

// Deliver writes one page. A full page returns status partial so the caller
// asks again with Cursor. A send error releases that user and returns
// ErrUnavailable; already written users stay claimed.
func (s *Service) Deliver(ctx context.Context, in Request) (Result, error) {
	if s == nil || s.Dir == nil || s.Claims == nil || s.Users == nil {
		return Result{}, ErrUnavailable
	}
	ids, err := s.Dir.Page(ctx, in.After, s.limit())
	if err != nil {
		return Result{}, ErrUnavailable
	}
	channel := s.postChannel(ctx, in)
	sent := 0
	for _, id := range ids {
		if Official(id) {
			continue
		}
		fresh, err := s.Claims.ClaimUser(ctx, in.EventID, id)
		if err != nil {
			return Result{}, ErrUnavailable
		}
		if !fresh {
			continue
		}
		if err := s.Users.Send(ctx, id, in.Text); err != nil {
			_ = s.Claims.ReleaseUser(ctx, in.EventID, id)
			return Result{}, ErrUnavailable
		}
		sent++
	}
	status := "done"
	if len(ids) == s.limit() {
		status = "partial"
	}
	cursor := in.After
	if len(ids) > 0 {
		cursor = ids[len(ids)-1]
	}
	return Result{Status: status, Cursor: cursor, Sent: sent, Channel: channel}, nil
}

func (s *Service) postChannel(ctx context.Context, in Request) string {
	if s.Channel == nil {
		return "skipped"
	}
	fresh, err := s.Claims.ClaimChannel(ctx, in.EventID)
	if err != nil {
		return "failed"
	}
	if !fresh {
		return "posted"
	}
	if err := s.Channel.Post(ctx, in.Text); err != nil {
		_ = s.Claims.ReleaseChannel(ctx, in.EventID)
		return "failed"
	}
	return "posted"
}

// Official reports an appendix D account that cannot sign in.
func Official(id int64) bool {
	return id >= ServiceUserID && id <= 777005
}

func validate(eventID, text, cursor string) (int64, error) {
	if !eventOK(eventID) || !textOK(text) {
		return 0, errBad
	}
	after, err := parseCursor(cursor)
	if err != nil {
		return 0, errBad
	}
	return after, nil
}

func eventOK(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == ':' || r == '-':
		default:
			return false
		}
	}
	return true
}

func textOK(text string) bool {
	text = strings.TrimSpace(text)
	n := utf8.RuneCountInString(text)
	if n < 1 || n > MaxTextRunes {
		return false
	}
	for _, r := range text {
		if r == 0x7f || (r < 0x20 && r != '\n' && r != '\t') {
			return false
		}
	}
	return true
}

func parseCursor(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0, nil
	}
	if len(s) > 1 && s[0] == '0' {
		return 0, errBad
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errBad
		}
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v < 0 {
		return 0, errBad
	}
	return v, nil
}
