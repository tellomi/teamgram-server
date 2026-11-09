package enforcement

import (
	"encoding/json"
	"net/http"
	"strings"
)

// ContentHandler is the content-engine review API. The bearer is
// TELLOMI_ENFORCEMENT_SERVICE_KEY, the same secret as credential recheck.
// A missing confirmer is HTTP 503. This handler does not ban an account.
type ContentHandler struct {
	Key []byte
}

func (h ContentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if !serviceAuthorized(r, h.Key) {
		writeDeniedStatus(w, http.StatusUnauthorized, "unauthorized", "unauthorized", "unauthorized")
		return
	}
	switch r.URL.Path {
	case "/internal/v1/enforcement/content/review":
		h.review(w, r)
	case "/internal/v1/enforcement/content/decision":
		h.decision(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h ContentHandler) review(w http.ResponseWriter, r *http.Request) {
	c := currentConfirmer()
	if c == nil {
		writeDeniedStatus(w, http.StatusServiceUnavailable, "disabled", "ENFORCEMENT_DISABLED", "disabled")
		return
	}
	var body ContentInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		writeDenied(w, http.StatusBadRequest, "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	if strings.TrimSpace(body.PostID) == "" && strings.TrimSpace(body.CommentID) == "" {
		writeDenied(w, http.StatusBadRequest, "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	out, err := c.ReviewContent(r.Context(), body)
	if err != nil {
		writeDeniedStatus(w, http.StatusServiceUnavailable, "unavailable", "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h ContentHandler) decision(w http.ResponseWriter, r *http.Request) {
	c := currentConfirmer()
	if c == nil {
		writeDeniedStatus(w, http.StatusServiceUnavailable, "disabled", "ENFORCEMENT_DISABLED", "disabled")
		return
	}
	var body struct {
		CaseID string `json:"case_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil || strings.TrimSpace(body.CaseID) == "" {
		writeDenied(w, http.StatusBadRequest, "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	out, ok, err := c.ContentStatus(body.CaseID)
	if err != nil {
		writeDeniedStatus(w, http.StatusServiceUnavailable, "unavailable", "ACCOUNT_STATE_UNKNOWN", stateUnknown)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
