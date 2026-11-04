package announce

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
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
	// TellomiAIUserID is @tellomi. Launch notices default to this sender.
	TellomiAIUserID int64 = 777001
	// EnvFrom overrides the default sender. Empty means TellomiAIUserID.
	// The value must be an official account, 777000 through 777005.
	EnvFrom = "TELLOMI_ANNOUNCE_FROM"
	// EnvGapMS is the pause after each private copy. Empty means 50.
	// 0 sends the page with no pause. Above 5000 is ignored.
	EnvGapMS = "TELLOMI_ANNOUNCE_GAP_MS"
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

// Users writes one private copy from Outbound.From.
type Users interface {
	Send(ctx context.Context, to int64, msg Outbound) error
}

// Channels writes the single @news post for an event.
type Channels interface {
	Post(ctx context.Context, msg Outbound) error
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
	// Gap is the pause after each successful private copy. Zero does not wait.
	Gap time.Duration
	// Wait replaces the timer. Tests use it so a gap does not sleep.
	Wait func(ctx context.Context, d time.Duration) error
}

// Request is one page. Cursor is the last user id already passed, or 0.
// From 0 uses DefaultSender (@tellomi, 777001).
type Request struct {
	EventID  string
	Text     string
	Entities []Entity
	From     int64
	After    int64
}

// Result is what the caller uses to continue.
// Channel is "posted", "failed", or "skipped".
type Result struct {
	Status  string
	Cursor  int64
	Sent    int
	Channel  string
	From     int64
	Preview  string
	Entities []Entity
	DryRun   bool
}

// DefaultSender is who a request uses when it omits from.
// An empty TELLOMI_ANNOUNCE_FROM is 777001. A value outside 777000–777005
// is refused by SenderFrom.
func DefaultSender() int64 {
	id, ok := SenderFrom(os.Getenv(EnvFrom))
	if !ok {
		return TellomiAIUserID
	}
	return id
}

// SenderFrom parses an optional sender. Empty is 777001 and ok.
// A bad value returns ok false.
func SenderFrom(raw string) (int64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return TellomiAIUserID, true
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || !AllowedSender(id) {
		return 0, false
	}
	return id, true
}

// AllowedSender is an official account that may originate a notice.
func AllowedSender(id int64) bool {
	return id >= ServiceUserID && id <= 777005
}

// GapFromEnv is the pause after each private copy.
func GapFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv(EnvGapMS))
	if raw == "" {
		return 50 * time.Millisecond
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n > 5000 {
		return 50 * time.Millisecond
	}
	return time.Duration(n) * time.Millisecond
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
	from := in.From
	if from == 0 {
		from = TellomiAIUserID
	}
	if !AllowedSender(from) {
		return Result{}, errBad
	}
	msg := Outbound{From: from, Text: in.Text, Entities: in.Entities}
	ids, err := s.Dir.Page(ctx, in.After, s.limit())
	if err != nil {
		return Result{}, ErrUnavailable
	}
	channel := s.postChannel(ctx, in, msg)
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
		if err := s.Users.Send(ctx, id, msg); err != nil {
			_ = s.Claims.ReleaseUser(ctx, in.EventID, id)
			return Result{}, ErrUnavailable
		}
		sent++
		if err := s.wait(ctx); err != nil {
			return Result{}, err
		}
	}
	status := "done"
	if len(ids) == s.limit() {
		status = "partial"
	}
	cursor := in.After
	if len(ids) > 0 {
		cursor = ids[len(ids)-1]
	}
	return Result{Status: status, Cursor: cursor, Sent: sent, Channel: channel, From: from}, nil
}

// Preview counts every recipient and returns the rendered text. It does not
// claim, send, or post to the channel.
func (s *Service) Preview(ctx context.Context, in Request) (Result, error) {
	if s == nil || s.Dir == nil {
		return Result{}, ErrUnavailable
	}
	from := in.From
	if from == 0 {
		from = TellomiAIUserID
	}
	if !AllowedSender(from) {
		return Result{}, errBad
	}
	var after int64
	var n int
	for {
		ids, err := s.Dir.Page(ctx, after, s.limit())
		if err != nil {
			return Result{}, ErrUnavailable
		}
		for _, id := range ids {
			if !Official(id) {
				n++
			}
		}
		if len(ids) < s.limit() {
			break
		}
		after = ids[len(ids)-1]
	}
	return Result{
		Status:   "dry_run",
		Sent:     n,
		From:     from,
		Preview:  in.Text,
		Entities: in.Entities,
		DryRun:   true,
		Channel:  "skipped",
	}, nil
}

func (s *Service) wait(ctx context.Context) error {
	if s == nil || s.Gap <= 0 {
		return nil
	}
	if s.Wait != nil {
		return s.Wait(ctx, s.Gap)
	}
	timer := time.NewTimer(s.Gap)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *Service) postChannel(ctx context.Context, in Request, msg Outbound) string {
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
	if err := s.Channel.Post(ctx, msg); err != nil {
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
