package enforcement

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/pkg/discoverauth"
)

const stateUnknown = "unknown"

// SubjectVerifier resolves the user inside a credential. It must ignore any
// user id supplied beside the credential. A lookup or store error is a failure,
// not an allow.
type SubjectVerifier interface {
	Subject(ctx context.Context, credential string, now time.Time) (userID int64, err error)
}

// CredentialHandler answers content-engine read/write checks.
// It does not call out to any URL in the request, and it does not accept a
// shared bearer as permission to name a user.
type CredentialHandler struct {
	Verify SubjectVerifier
	Now    func() time.Time
}

func (h CredentialHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/internal/v1/enforcement/credentials/check" {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= 0 {
		writeDenied(w, http.StatusGatewayTimeout, "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	var body struct {
		Credential string `json:"credential"`
		Write      bool   `json:"write"`
		UserID     int64  `json:"user_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil || strings.TrimSpace(body.Credential) == "" {
		writeDenied(w, http.StatusBadRequest, "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	now := time.Now()
	if h.Now != nil {
		now = h.Now()
	}
	if h.Verify == nil {
		writeDeniedStatus(w, http.StatusServiceUnavailable, "disabled", "ENFORCEMENT_DISABLED", "disabled")
		return
	}
	userID, err := h.Verify.Subject(ctx, body.Credential, now)
	if errors.Is(err, ErrSubjectUnbound) {
		writeDeniedStatus(w, http.StatusOK, "unbound", "SUBJECT_UNBOUND", "unbound")
		return
	}
	if err != nil || userID == 0 {
		writeDeniedStatus(w, http.StatusServiceUnavailable, "unavailable", "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	// body.UserID is intentionally unused. The credential subject is the only identity.
	d := CheckCredential(userID, body.Write, now)
	status, kind := http.StatusOK, "enforced"
	switch {
	case d.State == "disabled" || d.Code == "ENFORCEMENT_DISABLED":
		status, kind = http.StatusServiceUnavailable, "disabled"
		d.Allowed = false
	case d.Code == "ACCOUNT_STATE_UNKNOWN" || d.Degraded:
		status, kind = http.StatusServiceUnavailable, "unavailable"
		d.Allowed = false
	case d.Allowed:
		kind = "clear"
	}
	writeJSON(w, status, map[string]any{
		"allow":  d.Allowed,
		"status": kind,
		"code":   d.Code,
		"state":  d.State,
	})
}

// ErrSubjectUnbound means the JWT verified but no server-side sub binding exists.
// It is not a guess and not a store outage.
var ErrSubjectUnbound = errors.New("enforcement: credential subject is unbound")

func writeDenied(w http.ResponseWriter, status int, code, state string) {
	kind := "unavailable"
	if code == "ENFORCEMENT_DISABLED" {
		kind = "disabled"
	}
	writeDeniedStatus(w, status, kind, code, state)
}

func writeDeniedStatus(w http.ResponseWriter, status int, kind, code, state string) {
	writeJSON(w, status, map[string]any{"allow": false, "status": kind, "code": code, "state": state})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// FeedSubject verifies a discover credential with the existing HS256 key and
// resolves that subject through lookup. It does not accept a numeric user id
// from the caller and does not request any URL.
func FeedSubject(signingKey []byte, lookup func(sub string) (int64, bool, error), token string, now time.Time) (int64, error) {
	if len(signingKey) == 0 || lookup == nil {
		return 0, errors.New("enforcement: credential verifier is not configured")
	}
	claims, err := discoverauth.Verify(signingKey, token, now)
	if err != nil {
		return 0, err
	}
	userID, ok, err := lookup(claims.Sub)
	if err != nil {
		return 0, err
	}
	if !ok || userID == 0 {
		return 0, ErrSubjectUnbound
	}
	return userID, nil
}

// ListenLoopback serves the handler only on 127.0.0.1 or ::1.
// Any other address is refused so this process does not cleartext-bind a
// routable interface. TLS for a non-loopback hop is not implemented here.
func ListenLoopback(addr string, handler http.Handler) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host != "127.0.0.1" && host != "::1" {
		return errors.New("enforcement: listener must be loopback")
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	return srv.ListenAndServe()
}
