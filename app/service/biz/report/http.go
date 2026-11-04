package report

import (
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const (
	pathPending = "/internal/v1/reports/pending"
	maxBody     = 8 << 10
)

// Handler serves the human-review placeholder.
// POST /internal/v1/reports/{id}/decision
// GET  /internal/v1/reports/pending
type Handler struct {
	Service *Service
	Key     []byte
}

type decisionBody struct {
	Decision string `json:"decision"`
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !authorized(r, h.Key) {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.Service == nil {
		writeErr(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	if r.URL.Path == pathPending {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "method")
			return
		}
		h.pending(w, r)
		return
	}
	id, ok := decisionPath(r.URL.Path)
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method")
		return
	}
	var body decisionBody
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	verdict, ok := normalizeDecision(body.Decision)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	err := h.Service.Decide(r.Context(), id, verdict)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "ticket": Ticket(id), "status": verdict})
	case errors.Is(err, ErrNotFound):
		writeErr(w, http.StatusNotFound, "not_found")
	case errors.Is(err, ErrConflict):
		writeErr(w, http.StatusConflict, "conflict")
	case errors.Is(err, ErrBadDecision):
		writeErr(w, http.StatusBadRequest, "bad_request")
	default:
		writeErr(w, http.StatusServiceUnavailable, "unavailable")
	}
}

func (h Handler) pending(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeErr(w, http.StatusBadRequest, "bad_request")
			return
		}
		limit = n
	}
	rows, err := h.Service.Pending(r.Context(), limit)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"id":               row.ID,
			"ticket":           row.Ticket(),
			"source":           row.Source,
			"status":           row.Status,
			"target_key":       row.TargetKey,
			"reason":           row.Reason,
			"reporter_user_id": row.Reporter,
			"subject_user_id":  row.SubjectUserID,
			"created_at":       row.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"reports": out})
}

func decisionPath(path string) (int64, bool) {
	const prefix = "/internal/v1/reports/"
	const suffix = "/decision"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return 0, false
	}
	mid := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if mid == "" || strings.Contains(mid, "/") {
		return 0, false
	}
	id, err := strconv.ParseInt(mid, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func normalizeDecision(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case StatusViolation:
		return StatusViolation, true
	case StatusClear, "no_violation":
		return StatusClear, true
	default:
		return "", false
	}
}

func authorized(r *http.Request, key []byte) bool {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(header, prefix)
	if !ok || len(key) == 0 {
		return false
	}
	got, err := base64.StdEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil || len(got) != len(key) {
		return false
	}
	return hmac.Equal(got, key)
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
