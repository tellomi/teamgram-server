// Package ledger is the in-process wallet book.
//
// It stores Ton Connect sessions and wallet-card sticker placement. It does
// not create TON keys, derive receive addresses, or broadcast transfers.
package ledger

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// ProofDomain is the domain string returned by wallet.getProofChallenge.
	ProofDomain = "wallet.tellomi.local"
	maxSessions = 64
	maxStickers = 32

	CodeSessionInvalid   = "SESSION_ID_INVALID"
	CodeSessionClosed    = "SESSION_CLOSED"
	CodeSessionPending   = "SESSION_PENDING"
	CodeSessionConnected = "SESSION_ALREADY_CONNECTED"
	CodeChallengeInvalid = "CHALLENGE_INVALID"
	CodeDappInvalid      = "DAPP_CLIENT_ID_INVALID"
	CodeManifestInvalid  = "MANIFEST_URL_INVALID"
	CodeClientInvalid    = "CLIENT_ID_INVALID"
	CodeBodyEmpty        = "RESPONSE_BODY_EMPTY"
	CodeAnswerEmpty      = "CHALLENGE_ANSWER_EMPTY"
	CodeAppRequestEmpty  = "APP_REQUEST_ID_EMPTY"
	CodeRequestMissing   = "TONCONNECT_REQUEST_NOT_FOUND"
	CodeStickersTooMuch  = "STICKERS_TOO_MUCH"
	CodeOffsetInvalid    = "OFFSET_INVALID"
)

// Sticker is one sticker placed on the wallet card.
type Sticker struct {
	X, Y, W, H, Rotation float64
	Radius               float64
	HasRadius            bool
	NftAddress           string
}

// Card is the decoration stored for one user. Address and balance stay empty
// because this process does not operate a TON account.
type Card struct {
	Stickers  []Sticker
	BgAddress string
	HasBg     bool
}

// Session is one Ton Connect session owned by a user.
type Session struct {
	ID           int64
	DappClientID string
	ClientID     string
	ManifestURL  string
	Name         string
	Nonce        []byte
	Pending      bool
	Closing      bool
	Closed       bool
	Date         int32
	EventID      int64
	Challenge    []byte
	ConnectBody  []byte
	CloseBody    []byte
	Responses    map[int32][]byte
}

// Ledger is safe for concurrent use.
type Ledger struct {
	mu    sync.Mutex
	next  int64
	proof int64
	users map[int64]*userRec
}

type userRec struct {
	order    []int64
	sessions map[int64]*Session
	card     *Card
}

// New returns an empty ledger.
func New() *Ledger {
	return &Ledger{users: map[int64]*userRec{}, next: 1}
}

func (l *Ledger) rec(userID int64) *userRec {
	r := l.users[userID]
	if r == nil {
		r = &userRec{sessions: map[int64]*Session{}}
		l.users[userID] = r
	}
	return r
}

// Touch records that this user was read.
func (l *Ledger) Touch(userID int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rec(userID)
}

// ReadState reports that no chain wallet exists for the user.
func (l *Ledger) ReadState(userID int64) (created bool) {
	l.Touch(userID)
	return false
}

// Transactions accepts the first page and rejects any continuation offset.
func (l *Ledger) Transactions(userID int64, offset string) string {
	l.Touch(userID)
	if offset != "" {
		return CodeOffsetInvalid
	}
	return ""
}

// IssueProof returns a fresh ownership-challenge payload.
func (l *Ledger) IssueProof(userID int64, now time.Time) (payload string, expires int32) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rec(userID)
	l.proof++
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	payload = hex.EncodeToString(buf) + hex.EncodeToString([]byte{byte(l.proof)})
	return payload, int32(now.Add(10 * time.Minute).Unix())
}

// SetCard replaces the user's card decoration.
func (l *Ledger) SetCard(userID int64, card Card) string {
	if len(card.Stickers) > maxStickers {
		return CodeStickersTooMuch
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	r := l.rec(userID)
	cp := card
	cp.Stickers = append([]Sticker(nil), card.Stickers...)
	r.card = &cp
	return ""
}

// Card returns the stored decoration.
func (l *Ledger) Card(userID int64) (Card, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	r := l.users[userID]
	if r == nil || r.card == nil {
		return Card{}, false
	}
	cp := *r.card
	cp.Stickers = append([]Sticker(nil), r.card.Stickers...)
	return cp, true
}

// CreateSession opens a session, or returns the open one for the same dapp.
func (l *Ledger) CreateSession(userID int64, dapp, manifestURL string, now time.Time) (Session, string) {
	dapp = strings.TrimSpace(dapp)
	if dapp == "" || len(dapp) > 256 {
		return Session{}, CodeDappInvalid
	}
	name, code := manifestName(manifestURL)
	if code != "" {
		return Session{}, code
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	r := l.rec(userID)
	for i := len(r.order) - 1; i >= 0; i-- {
		s := r.sessions[r.order[i]]
		if s != nil && s.DappClientID == dapp && !s.Closed {
			return s.clone(), ""
		}
	}
	l.evict(r)
	id := l.next
	l.next++
	s := &Session{
		ID:           id,
		DappClientID: dapp,
		ManifestURL:  manifestURL,
		Name:         name,
		Nonce:        random(24),
		Pending:      true,
		Date:         int32(now.Unix()),
	}
	r.sessions[id] = s
	r.order = append(r.order, id)
	return s.clone(), ""
}

func (l *Ledger) evict(r *userRec) {
	for len(r.order) >= maxSessions {
		drop := 0
		for i, id := range r.order {
			if s := r.sessions[id]; s != nil && s.Closed {
				drop = i
				break
			}
		}
		id := r.order[drop]
		delete(r.sessions, id)
		r.order = append(r.order[:drop], r.order[drop+1:]...)
	}
}

// RegisterKey issues a connect challenge for a pending session.
func (l *Ledger) RegisterKey(userID, sessionID int64, clientID string) (challenge []byte, eventID int64, code string) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" || len(clientID) > 256 {
		return nil, 0, CodeClientInvalid
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	s, code := l.session(userID, sessionID)
	if code != "" {
		return nil, 0, code
	}
	if s.Closed {
		return nil, 0, CodeSessionClosed
	}
	if !s.Pending {
		return nil, 0, CodeSessionConnected
	}
	s.ClientID = clientID
	s.Challenge = random(32)
	s.EventID++
	return append([]byte(nil), s.Challenge...), s.EventID, ""
}

// SubmitConnect stores a connect result. An error flag closes the session.
// The body is not forwarded to a dapp bridge.
func (l *Ledger) SubmitConnect(userID, sessionID int64, isErr bool, answer, body []byte) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, code := l.session(userID, sessionID)
	if code != "" {
		return code
	}
	if s.Closed {
		return CodeSessionClosed
	}
	if isErr {
		s.Pending = false
		s.Closed = true
		s.ConnectBody = append([]byte(nil), body...)
		return ""
	}
	if len(s.Challenge) == 0 {
		return CodeChallengeInvalid
	}
	if len(answer) == 0 {
		return CodeAnswerEmpty
	}
	if len(body) == 0 {
		return CodeBodyEmpty
	}
	s.Pending = false
	s.ConnectBody = append([]byte(nil), body...)
	return ""
}

// Pending returns one session. sessionID, when non-nil, wins over dapp.
func (l *Ledger) Pending(userID int64, sessionID *int64, dapp string) (Session, string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	r := l.users[userID]
	if sessionID != nil {
		s, code := l.session(userID, *sessionID)
		if code != "" {
			return Session{}, code
		}
		return s.clone(), ""
	}
	if r == nil {
		return Session{}, CodeSessionInvalid
	}
	for i := len(r.order) - 1; i >= 0; i-- {
		s := r.sessions[r.order[i]]
		if s == nil || s.Closed {
			continue
		}
		if dapp != "" && s.DappClientID != dapp {
			continue
		}
		return s.clone(), ""
	}
	return Session{}, CodeSessionInvalid
}

// Claim reports that no dapp request is waiting. The bridge is not connected.
func (l *Ledger) Claim(userID, sessionID int64, appRequestID string) string {
	if strings.TrimSpace(appRequestID) == "" {
		return CodeAppRequestEmpty
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	s, code := l.session(userID, sessionID)
	if code != "" {
		return code
	}
	if s.Closed {
		return CodeSessionClosed
	}
	return CodeRequestMissing
}

// SubmitResponse stores a response body for a connected session.
func (l *Ledger) SubmitResponse(userID, sessionID int64, msgID int32, body []byte) string {
	if len(body) == 0 {
		return CodeBodyEmpty
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	s, code := l.session(userID, sessionID)
	if code != "" {
		return code
	}
	if s.Closed {
		return CodeSessionClosed
	}
	if s.Pending {
		return CodeSessionPending
	}
	if s.Responses == nil {
		s.Responses = map[int32][]byte{}
	}
	s.Responses[msgID] = append([]byte(nil), body...)
	return ""
}

// StoredResponse is the body recorded by SubmitResponse.
func (l *Ledger) StoredResponse(userID, sessionID int64, msgID int32) ([]byte, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, code := l.session(userID, sessionID)
	if code != "" || s.Responses == nil {
		return nil, false
	}
	b, ok := s.Responses[msgID]
	return append([]byte(nil), b...), ok
}

// NextEvent allocates the next event id for a session.
func (l *Ledger) NextEvent(userID, sessionID int64) (int64, string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, code := l.session(userID, sessionID)
	if code != "" {
		return 0, code
	}
	if s.Closed {
		return 0, CodeSessionClosed
	}
	s.EventID++
	return s.EventID, ""
}

// Close marks a session closed. Closing an unknown id fails. Closing twice succeeds.
func (l *Ledger) Close(userID, sessionID int64, body []byte) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, code := l.session(userID, sessionID)
	if code != "" {
		return code
	}
	s.Pending = false
	s.Closing = false
	s.Closed = true
	if len(body) > 0 {
		s.CloseBody = append([]byte(nil), body...)
	}
	return ""
}

// Sessions lists sessions newest first.
func (l *Ledger) Sessions(userID int64) []Session {
	l.mu.Lock()
	defer l.mu.Unlock()
	r := l.users[userID]
	if r == nil {
		return []Session{}
	}
	out := make([]Session, 0, len(r.order))
	for i := len(r.order) - 1; i >= 0; i-- {
		if s := r.sessions[r.order[i]]; s != nil {
			out = append(out, s.clone())
		}
	}
	return out
}

func (l *Ledger) session(userID, sessionID int64) (*Session, string) {
	r := l.users[userID]
	if r == nil || sessionID == 0 {
		return nil, CodeSessionInvalid
	}
	s := r.sessions[sessionID]
	if s == nil {
		return nil, CodeSessionInvalid
	}
	return s, ""
}

func (s *Session) clone() Session {
	out := *s
	out.Nonce = append([]byte(nil), s.Nonce...)
	out.Challenge = append([]byte(nil), s.Challenge...)
	out.ConnectBody = append([]byte(nil), s.ConnectBody...)
	out.CloseBody = append([]byte(nil), s.CloseBody...)
	if s.Responses != nil {
		out.Responses = make(map[int32][]byte, len(s.Responses))
		for k, v := range s.Responses {
			out.Responses[k] = append([]byte(nil), v...)
		}
	}
	return out
}

func manifestName(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 {
		return "", CodeManifestInvalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", CodeManifestInvalid
	}
	return u.Host, ""
}

func random(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		for i := range b {
			b[i] = byte(i + 1)
		}
	}
	return b
}
