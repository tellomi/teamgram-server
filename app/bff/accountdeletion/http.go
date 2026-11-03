// Package accountdeletion is the web form that deletes a Tellomi account.
// Phone mode sends SMS through the same provider as auth.sendCode. Email mode
// sends through the same sender as email login (TELLOMI_EMAIL_PROVIDER).
// A cloud password is checked with the same SRP verifier, and the account is
// deleted through account.deleteAccount.
package accountdeletion

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/teamgram-server/pkg/phonenumber"
	"github.com/teamgram/teamgram-server/pkg/verify"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	sessionTTL     = 10 * time.Minute
	maxBody        = 8 << 10
	maxAttempts    = 5
	confirmWordEN  = "DELETE"
	confirmWordZH  = "删除"
	kindSMS        = "sms"
	kindEmail      = "email"
	defaultCodeLen = 6
)

var digitRun = regexp.MustCompile(`[0-9]{4,}`)

// Account is the user behind a phone or an email address, after lookup.
type Account struct {
	UserID   int64
	Username string
	Phone    string
}

// Deps are the Teamgram services the form is allowed to call.
type Deps struct {
	// Lookup returns the account for a normalized phone or email. ok is false
	// when nobody has that identifier. The HTTP response does not reveal which.
	// An email is stored on users.email; user.getImmutableUserByPhone reads
	// that column when the argument contains "@".
	Lookup func(ctx context.Context, phoneOrEmail string) (Account, bool, error)
	// Password reports a cloud password. setAt 0 means the age is unknown.
	Password func(ctx context.Context, userID int64) (has bool, setAt int64, hint string, err error)
	// CheckPassword accepts a password typed on the form.
	CheckPassword func(ctx context.Context, userID int64, plain string) error
	// Purge is account.deleteAccount's cleanup.
	Purge func(ctx context.Context, userID int64, reason string) error
	// SendSMS delivers a phone code. Nil in tests that only check the HTTP shape.
	SendSMS func(ctx context.Context, phone, code string) error
	// SendEmail delivers an email code through the login email sender.
	// Nil in tests that only check the HTTP shape. A failure is logged and
	// answered like a missing account.
	SendEmail func(ctx context.Context, email, code string) error
	// AllowSend is the send quota. kind is "sms" or "email". Nil allows the send.
	AllowSend func(ctx context.Context, kind, target, ip string) error
	Now       func() time.Time
	// FixedCode is the development phone code when SMS is not really sent.
	// Empty means generate a random code and call SendSMS. It is never used
	// for email.
	FixedCode string
	// FixedEmailCode, when set, skips SendEmail and accepts this code.
	// Production leaves it empty so a random code is mailed (or logged).
	FixedEmailCode string
	CodeLen        int
	EmailCodeLen   int
	Origins        []string
}

type session struct {
	Phone     string
	Email     string
	Kind      string
	UserID    int64
	CodeHash  [32]byte
	Expires   time.Time
	Attempts  int
	PassFails int
	Verified  bool
	Used      bool
	HasPass   bool
	Hint      string
}

type memSessions struct {
	mu   sync.Mutex
	rows map[string]*session
}

func newMemSessions() *memSessions {
	return &memSessions{rows: map[string]*session{}}
}

func (m *memSessions) put(id string, s *session) {
	m.mu.Lock()
	m.rows[id] = s
	m.mu.Unlock()
}

func (m *memSessions) get(id string) (*session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.rows[id]
	if !ok {
		return nil, false
	}
	cp := *s
	return &cp, true
}

func (m *memSessions) update(id string, fn func(*session)) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.rows[id]
	if !ok {
		return false
	}
	fn(s)
	return true
}

// Server is the deletion HTTP API and the form page.
type Server struct {
	deps     Deps
	sessions *memSessions
	origins  map[string]struct{}
}

func New(deps Deps) *Server {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.CodeLen <= 0 {
		deps.CodeLen = defaultCodeLen
	}
	if deps.EmailCodeLen <= 0 {
		deps.EmailCodeLen = defaultCodeLen
	}
	origins := map[string]struct{}{}
	list := deps.Origins
	if len(list) == 0 {
		list = []string{"https://www.tellomi.cn", "https://chat.tellomi.cn"}
	}
	for _, o := range list {
		origins[strings.TrimRight(o, "/")] = struct{}{}
	}
	return &Server{deps: deps, sessions: newMemSessions(), origins: origins}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/verification/session", s.session)
	mux.HandleFunc("/v1/tellomi/account-deletion", s.delete)
	mux.HandleFunc("/delete-account", s.page)
	mux.HandleFunc("/delete-account/", s.page)
	return mux
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	if !s.cors(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method")
		return
	}
	var body struct {
		Phone       string `json:"phone"`
		PhoneNumber string `json:"phone_number"`
		Email       string `json:"email"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	tgt, errCode := s.resolveTarget(body.Phone, body.PhoneNumber, body.Email)
	if errCode != "" {
		writeErr(w, http.StatusBadRequest, errCode)
		return
	}
	ip := clientIP(r)
	acc, found, err := s.deps.Lookup(r.Context(), tgt.Value)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "unavailable")
		return
	}
	// A missing account must not consume that phone or email's quota. The IP
	// quota still applies, under a key that is not a real identifier.
	quotaTarget := tgt.Value
	if !found {
		quotaTarget = "absent:" + ip
	}
	if s.deps.AllowSend != nil {
		if err = s.deps.AllowSend(r.Context(), tgt.Kind, quotaTarget, ip); err != nil {
			var lim *verify.LimitError
			if errors.As(err, &lim) {
				writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate_limited", "retry_after": lim.Retry})
				return
			}
			writeErr(w, http.StatusTooManyRequests, "rate_limited")
			return
		}
	}
	code, fixed := s.issueCode(tgt)
	send := s.deps.SendSMS
	if tgt.Kind == kindEmail {
		send = s.deps.SendEmail
	}
	if found && send != nil && !fixed {
		ctx, cancel := context.WithTimeout(r.Context(), verify.SMSSendTimeout)
		err = send(ctx, tgt.Value, code)
		cancel()
		if err != nil {
			// Same response as a missing account. Digits in the provider
			// error are stripped so a code cannot land in the log.
			logx.Errorf("account deletion %s failed: %s", tgt.Kind, RedactDigits(err.Error()))
			found = false
		}
	}
	id := newID()
	row := &session{
		Kind:     tgt.Kind,
		Expires:  s.deps.Now().Add(sessionTTL),
		CodeHash: sha256.Sum256([]byte(code)),
	}
	if tgt.Kind == kindEmail {
		row.Email = tgt.Value
	} else {
		row.Phone = tgt.Value
	}
	if found {
		row.UserID = acc.UserID
		has, _, hint, perr := s.deps.Password(r.Context(), acc.UserID)
		if perr != nil {
			writeErr(w, http.StatusInternalServerError, "unavailable")
			return
		}
		row.HasPass = has
		row.Hint = hint
	} else {
		// Unusable code. The HTTP body matches a real send.
		row.CodeHash = sha256.Sum256([]byte(newID()))
	}
	s.sessions.put(id, row)
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id":  id,
		"expires_in":  int(sessionTTL / time.Second),
		"code_length": len(code),
	})
}

func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	if !s.cors(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method")
		return
	}
	var body struct {
		SessionID string `json:"session_id"`
		Code      string `json:"code"`
		Password  string `json:"password"`
		Confirm   string `json:"confirm"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	row, ok := s.sessions.get(body.SessionID)
	if !ok || row.Used || s.deps.Now().After(row.Expires) {
		writeErr(w, http.StatusBadRequest, "code_invalid")
		return
	}
	if !row.Verified {
		sum := sha256.Sum256([]byte(strings.TrimSpace(body.Code)))
		if subtle.ConstantTimeCompare(sum[:], row.CodeHash[:]) != 1 {
			dead := false
			s.sessions.update(body.SessionID, func(s *session) {
				s.Attempts++
				if s.Attempts >= maxAttempts {
					s.Used = true
					dead = true
				}
			})
			_ = dead
			writeErr(w, http.StatusBadRequest, "code_invalid")
			return
		}
		s.sessions.update(body.SessionID, func(s *session) { s.Verified = true })
		row.Verified = true
	}
	if row.UserID == 0 {
		s.sessions.update(body.SessionID, func(s *session) { s.Used = true })
		writeErr(w, http.StatusBadRequest, "code_invalid")
		return
	}
	if row.HasPass && body.Password == "" {
		resp := map[string]any{"error": "password_required"}
		if row.Hint != "" {
			resp["hint"] = row.Hint
		}
		writeJSON(w, http.StatusConflict, resp)
		return
	}
	if row.HasPass {
		if err := s.deps.CheckPassword(r.Context(), row.UserID, body.Password); err != nil {
			s.sessions.update(body.SessionID, func(s *session) {
				s.PassFails++
				if s.PassFails >= maxAttempts {
					s.Used = true
				}
			})
			writeErr(w, http.StatusBadRequest, "password_invalid")
			return
		}
	}
	if !confirmAccepted(body.Confirm) {
		writeErr(w, http.StatusBadRequest, "confirm_required")
		return
	}
	if err := s.deps.Purge(r.Context(), row.UserID, "web"); err != nil {
		writeErr(w, http.StatusInternalServerError, "unavailable")
		return
	}
	s.sessions.update(body.SessionID, func(s *session) { s.Used = true })
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeErr(w, http.StatusMethodNotAllowed, "method")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", pageCSP)
	_, _ = w.Write(pageHTML)
}

// confirmAccepted is the exact word for either locale. The page asks for
// 删除 in Chinese and DELETE in English. Nothing else matches, including
// a different case or surrounding space.
func confirmAccepted(v string) bool {
	return v == confirmWordEN || v == confirmWordZH
}

type deliveryTarget struct {
	Kind    string
	Value   string
	CodeLen int
}

func (s *Server) resolveTarget(phone, phoneNumber, email string) (deliveryTarget, string) {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		phone = strings.TrimSpace(phoneNumber)
	}
	email = strings.TrimSpace(email)
	switch {
	case email != "" && phone != "":
		return deliveryTarget{}, "bad_request"
	case email != "":
		email = verify.NormalizeEmail(email)
		if !verify.ValidEmail(email) || len(email) > 255 {
			return deliveryTarget{}, "email_invalid"
		}
		return deliveryTarget{Kind: kindEmail, Value: email, CodeLen: s.deps.EmailCodeLen}, ""
	default:
		normalized, ok := normalizePhone(phone)
		if !ok {
			return deliveryTarget{}, "phone_invalid"
		}
		return deliveryTarget{Kind: kindSMS, Value: normalized, CodeLen: s.deps.CodeLen}, ""
	}
}

func (s *Server) issueCode(tgt deliveryTarget) (code string, fixed bool) {
	if tgt.Kind == kindEmail {
		code = s.deps.FixedEmailCode
	} else {
		code = s.deps.FixedCode
	}
	if code != "" {
		return code, true
	}
	n := tgt.CodeLen
	if n <= 0 {
		n = defaultCodeLen
	}
	return verify.RandomDigits(n), false
}

func (s *Server) cors(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if _, ok := s.origins[origin]; !ok {
		writeErr(w, http.StatusForbidden, "origin")
		return false
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Vary", "Origin")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	return true
}

func normalizePhone(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if !strings.HasPrefix(raw, "+") {
		raw = "+" + raw
	}
	region, digits, err := phonenumber.CheckPhoneNumberInvalid(raw)
	if err != nil || digits == "" {
		return "", false
	}
	if region != "" && region != "CN" && !verify.Load().SMS.AllowInternational {
		return "", false
	}
	return digits, true
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		first, _, _ := strings.Cut(fwd, ",")
		first = strings.TrimSpace(first)
		if ip := net.ParseIP(first); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	return dec.Decode(dst)
}

func writeErr(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// RedactDigits hides codes that might have leaked into an error string.
func RedactDigits(s string) string {
	return digitRun.ReplaceAllString(s, "****")
}
