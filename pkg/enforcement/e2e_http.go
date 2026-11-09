package enforcement

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"
)

// E2ELoopbackAddr is the isolated listener used by the client regression job.
// It is inside the teamgram container and is not published by the default
// compose file.
const E2ELoopbackAddr = "127.0.0.1:8819"

// EnvE2E turns on that listener. Any other value, including empty, leaves it
// off. It is not a production switch.
const EnvE2E = "TELLOMI_ENFORCEMENT_E2E"

// ServeIsolatedE2E listens on a loopback address for the disposition routes
// and the synthetic-account operator. It refuses a routable address, an unset
// flag, and a missing service key.
func ServeIsolatedE2E(addr string) error {
	if os.Getenv(EnvE2E) != "1" {
		return errors.New("enforcement: isolated e2e listener is off")
	}
	key, err := LoadServiceKey()
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/internal/v1/enforcement/disposition/", DispositionHandler{})
	mux.Handle("/internal/v1/enforcement/e2e/", E2EHandler{Key: key})
	return ListenLoopback(addr, mux)
}

// E2EHandler creates, approves, revokes, and inspects cases for the isolated
// client regression. The service key is the same bearer as credential recheck.
// A missing or wrong key is HTTP 401 with one body. This handler is not mounted
// unless ServeIsolatedE2E runs.
type E2EHandler struct {
	Key []byte
	Now func() time.Time
}

func (h E2EHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if !serviceAuthorized(r, h.Key) {
		writeDeniedStatus(w, http.StatusUnauthorized, "unauthorized", "unauthorized", "unauthorized")
		return
	}
	s := Current()
	if s == nil {
		writeDeniedStatus(w, http.StatusServiceUnavailable, "disabled", "ENFORCEMENT_DISABLED", "disabled")
		return
	}
	now := time.Now()
	if h.Now != nil {
		now = h.Now()
	}
	switch r.URL.Path {
	case "/internal/v1/enforcement/e2e/cases":
		h.create(w, r, s, now)
	case "/internal/v1/enforcement/e2e/revoke":
		h.revoke(w, r, s)
	case "/internal/v1/enforcement/e2e/stash":
		h.stash(w, r, s, now)
	case "/internal/v1/enforcement/e2e/inspect":
		h.inspect(w, r, s)
	case "/internal/v1/enforcement/e2e/flush":
		s.Flush()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

func (h E2EHandler) create(w http.ResponseWriter, r *http.Request, s *Service, now time.Time) {
	var body struct {
		UserID int64    `json:"user_id"`
		Kind   string   `json:"kind"`
		Scopes []string `json:"scopes"`
		Phone  string   `json:"phone"`
		Email  string   `json:"email"`
		Hours  int      `json:"hours"`
		Key    string   `json:"idempotency_key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil || body.UserID == 0 || body.Kind == "" {
		writeDenied(w, http.StatusBadRequest, "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	in := Sanction{
		UserID: body.UserID, Kind: body.Kind, Scopes: body.Scopes,
		ReasonCategory: "合成", PolicyClause: "隔离条款", RuleVersion: "e2e-1",
		EvidenceRef: "e2e", IdempotencyKey: body.Key, Start: now,
	}
	if body.Kind == KindPermanent {
		in.Permanent = true
	} else {
		hours := body.Hours
		if hours <= 0 {
			hours = 2
		}
		end := now.Add(time.Duration(hours) * time.Hour)
		in.End = &end
	}
	row, err := s.Create(Actor{ID: "e2e-s1", Role: RoleSenior}, in)
	if err != nil {
		writeDenied(w, http.StatusServiceUnavailable, "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	if row.Status == StatusPendingApproval {
		row, err = s.Approve(Actor{ID: "e2e-s2", Role: RoleSenior}, row.ID)
		if err != nil {
			writeDenied(w, http.StatusServiceUnavailable, "ACCOUNT_STATE_UNKNOWN", stateUnknown)
			return
		}
	}
	if body.Phone != "" {
		s.BindIdentifier(body.UserID, "phone", body.Phone)
	}
	if body.Email != "" {
		s.BindIdentifier(body.UserID, "email", body.Email)
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": row.ID, "status": row.Status, "user_id": row.UserID})
}

func (h E2EHandler) revoke(w http.ResponseWriter, r *http.Request, s *Service) {
	var body struct {
		ID     string `json:"id"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil || body.ID == "" || body.Reason == "" {
		writeDenied(w, http.StatusBadRequest, "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	row, err := s.Revoke(Actor{ID: "e2e-s3", Role: RoleSenior}, body.ID, body.Reason)
	if err != nil {
		writeDenied(w, http.StatusForbidden, "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": row.ID, "status": row.Status})
}

func (h E2EHandler) stash(w http.ResponseWriter, r *http.Request, s *Service, now time.Time) {
	var body struct {
		UserID int64  `json:"user_id"`
		Proof  string `json:"proof"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil || body.UserID == 0 || body.Proof == "" {
		writeDenied(w, http.StatusBadRequest, "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	s.mu.Lock()
	if err := s.begin(); err != nil {
		s.mu.Unlock()
		writeDenied(w, http.StatusServiceUnavailable, "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	grant := s.issueLocked(body.UserID, now)
	if err := s.finish(nil); err != nil {
		s.mu.Unlock()
		writeDenied(w, http.StatusServiceUnavailable, "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	s.mu.Unlock()
	if err := s.StashProof(body.Proof, grant); err != nil {
		writeDenied(w, http.StatusServiceUnavailable, "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h E2EHandler) inspect(w http.ResponseWriter, r *http.Request, s *Service) {
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil || body.ID == "" {
		writeDenied(w, http.StatusBadRequest, "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	s.mu.Lock()
	if err := s.begin(); err != nil {
		s.mu.Unlock()
		writeDenied(w, http.StatusServiceUnavailable, "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	row := s.rows[body.ID]
	if row == nil {
		_ = s.finish(nil)
		s.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	out := map[string]any{"id": row.ID, "status": row.Status, "user_id": row.UserID, "kind": row.Kind}
	var notices []map[string]any
	for _, key := range s.noticeOrder {
		n := s.notices[key]
		if n == nil || !strings.HasPrefix(key, body.ID+":") {
			continue
		}
		notices = append(notices, map[string]any{
			"key": key, "from": n.From, "template": n.Template, "status": n.Status, "attempts": n.Attempts, "body": n.Body,
		})
	}
	var audits []map[string]any
	for _, a := range s.audits {
		if a.CaseID != body.ID && !strings.HasPrefix(a.CaseID, body.ID+":") {
			continue
		}
		audits = append(audits, map[string]any{"actor": a.Actor, "action": a.Action, "case_id": a.CaseID, "detail": a.Detail})
	}
	var idents []map[string]string
	for key, id := range s.bind {
		if id != row.UserID {
			continue
		}
		kind, value, _ := splitBind(key)
		idents = append(idents, map[string]string{"kind": kind, "value": value})
	}
	_ = s.finish(nil)
	s.mu.Unlock()
	out["notices"] = notices
	out["audits"] = audits
	out["identifiers"] = idents
	writeJSON(w, http.StatusOK, out)
}

func splitBind(key string) (string, string, bool) {
	for i := 0; i < len(key); i++ {
		if key[i] == 0 {
			return key[:i], key[i+1:], true
		}
	}
	return "", "", false
}
