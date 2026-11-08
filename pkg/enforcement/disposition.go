package enforcement

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// RememberLoginProof keeps the verified phone_code_hash for this auth key
// until checkPassword finishes. It does not by itself issue a grant.
func (s *Service) RememberLoginProof(authKey int64, proof string) error {
	if s == nil || authKey == 0 || proof == "" {
		return ErrNotFound
	}
	if s.db == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.loginProof == nil {
			s.loginProof = map[int64]string{}
		}
		s.loginProof[authKey] = proof
		return nil
	}
	_, err := s.db.Exec(`INSERT INTO login_proof (auth_key, proof) VALUES (?,?) ON DUPLICATE KEY UPDATE proof=VALUES(proof)`, authKey, proof)
	return err
}

// TakeLoginProof returns the hash saved for this auth key.
func (s *Service) TakeLoginProof(authKey int64) string {
	if s == nil || authKey == 0 {
		return ""
	}
	if s.db == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.loginProof[authKey]
	}
	var proof string
	_ = s.db.QueryRow(`SELECT proof FROM login_proof WHERE auth_key = ?`, authKey).Scan(&proof)
	return proof
}

// StashProof stores a one-time claim for a disposition grant. The proof is the
// phone_code_hash the client already holds from the verified login. The user
// id is taken from the grant, never from a later request body.
func (s *Service) StashProof(proof string, g Grant) error {
	if s == nil || proof == "" || g.Token == "" || g.UserID == 0 {
		return ErrNotFound
	}
	if s.db == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		cp := g
		s.claims[proof] = &cp
		s.grants[g.Token] = &cp
		return nil
	}
	_, err := s.db.Exec(`INSERT INTO disposition_grant (token, user_id, expires_at) VALUES (?,?,?)
		ON DUPLICATE KEY UPDATE expires_at=VALUES(expires_at), user_id=VALUES(user_id)`, g.Token, g.UserID, g.Expires.UTC())
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO disposition_claim (proof, token, user_id, expires_at, used) VALUES (?,?,?,?,0)`,
		proof, g.Token, g.UserID, g.Expires.UTC())
	return err
}

// Claim returns the grant once. A second call for the same proof fails.
func (s *Service) Claim(proof string, now time.Time) (Grant, error) {
	if s == nil || proof == "" {
		return Grant{}, ErrNotFound
	}
	if s.db == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		g := s.claims[proof]
		if g == nil || !now.Before(g.Expires) {
			return Grant{}, ErrNotFound
		}
		delete(s.claims, proof)
		return *g, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Grant{}, err
	}
	defer tx.Rollback()
	var g Grant
	err = tx.QueryRow(`SELECT token, user_id, expires_at FROM disposition_claim WHERE proof = ? AND used = 0 FOR UPDATE`, proof).
		Scan(&g.Token, &g.UserID, &g.Expires)
	if err == sql.ErrNoRows || !now.Before(g.Expires) {
		return Grant{}, ErrNotFound
	}
	if err != nil {
		return Grant{}, err
	}
	if _, err = tx.Exec(`UPDATE disposition_claim SET used = 1 WHERE proof = ?`, proof); err != nil {
		return Grant{}, err
	}
	if err = tx.Commit(); err != nil {
		return Grant{}, err
	}
	return g, nil
}

// DispositionHandler serves the restricted notice and appeal routes.
// Identity is the disposition grant. A user_id in the body is ignored.
type DispositionHandler struct {
	Now func() time.Time
}

func (h DispositionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s := Current()
	if s == nil {
		writeDenied(w, http.StatusServiceUnavailable, "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	now := time.Now()
	if h.Now != nil {
		now = h.Now()
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/internal/v1/enforcement/disposition/claim":
		h.claim(w, r, s, now)
	case r.Method == http.MethodPost && r.URL.Path == "/internal/v1/enforcement/disposition/notices":
		h.notices(w, r, s, now)
	case r.Method == http.MethodPost && r.URL.Path == "/internal/v1/enforcement/disposition/appeals":
		h.appeal(w, r, s, now)
	default:
		http.NotFound(w, r)
	}
}

func (h DispositionHandler) claim(w http.ResponseWriter, r *http.Request, s *Service, now time.Time) {
	var body struct {
		Proof  string `json:"proof"`
		UserID int64  `json:"user_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil || body.Proof == "" {
		writeDenied(w, http.StatusBadRequest, "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	g, err := s.Claim(body.Proof, now)
	if err != nil {
		writeDenied(w, http.StatusUnauthorized, "ACCOUNT_PERMANENT_DISABLED", KindPermanent)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": g.Token, "expires": g.Expires.UTC().Format(time.RFC3339)})
}

func grantFromRequest(r *http.Request, s *Service, now time.Time) (Grant, bool) {
	const prefix = "Disposition "
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return Grant{}, false
	}
	g, ok := s.UseGrant(strings.TrimPrefix(header, prefix), now)
	return g, ok
}

func (h DispositionHandler) notices(w http.ResponseWriter, r *http.Request, s *Service, now time.Time) {
	g, ok := grantFromRequest(r, s, now)
	if !ok {
		writeDenied(w, http.StatusUnauthorized, "ACCOUNT_PERMANENT_DISABLED", KindPermanent)
		return
	}
	notes, err := s.NoticesForGrant(g.Token, now)
	if err != nil {
		writeDenied(w, http.StatusServiceUnavailable, "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	out := make([]map[string]any, 0, len(notes))
	for _, n := range notes {
		if n.UserID != g.UserID || n.From != AdminUserID {
			continue
		}
		out = append(out, map[string]any{"from": n.From, "template": n.Template, "body": n.Body})
	}
	writeJSON(w, http.StatusOK, map[string]any{"notices": out})
}

func (h DispositionHandler) appeal(w http.ResponseWriter, r *http.Request, s *Service, now time.Time) {
	g, ok := grantFromRequest(r, s, now)
	if !ok {
		writeDenied(w, http.StatusUnauthorized, "ACCOUNT_PERMANENT_DISABLED", KindPermanent)
		return
	}
	var body struct {
		SanctionID string `json:"sanction_id"`
		Text       string `json:"text"`
		UserID     int64  `json:"user_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil || body.SanctionID == "" {
		writeDenied(w, http.StatusBadRequest, "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	appeal, err := s.SubmitAppealWithGrant(g.Token, body.SanctionID, body.Text, now)
	if err != nil {
		writeDenied(w, http.StatusForbidden, "ACCOUNT_PERMANENT_DISABLED", KindPermanent)
		return
	}
	if appeal.UserID != g.UserID {
		writeDenied(w, http.StatusForbidden, "ACCOUNT_PERMANENT_DISABLED", KindPermanent)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"appeal_id": appeal.ID, "status": appeal.Status})
}
