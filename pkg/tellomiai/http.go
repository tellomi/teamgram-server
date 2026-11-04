package tellomiai

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

const (
	// EnvInternalKey is standard base64 of at least 32 bytes. The engine
	// sends that same base64 as Authorization: Bearer.
	EnvInternalKey = "TELLOMI_AI_INTERNAL_KEY"
	// EnvAddr is the listen address. Empty means 127.0.0.1:8814. "off" does not listen.
	EnvAddr      = "TELLOMI_AI_ADDR"
	defaultAddr  = "127.0.0.1:8814"
	maxBody      = 64 << 10
	pathComplete = "/internal/v1/ai/complete"
)

// Subjects resolves a content-engine sub (16 raw bytes) to a user id.
type Subjects interface {
	Users(ctx context.Context, subs [][]byte) (map[string]int64, error)
}

// Handler is POST /internal/v1/ai/complete. Phase 1: the engine parsed
// @tellomi and wants the reply text to write as that account's comment.
type Handler struct {
	Service  *Service
	Subjects Subjects
	Key      []byte
}

type completeRequest struct {
	EventID string `json:"event_id"`
	UserSub string `json:"user_sub"`
	Text    string `json:"text"`
}

// ServeHTTP answers one mention. The engine writes the comment; this
// process does not.
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != pathComplete {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method")
		return
	}
	if !authorized(r, h.Key) {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body completeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if !eventID(body.EventID) || strings.TrimSpace(body.Text) == "" {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	sub, err := decodeSub(body.UserSub)
	if err != nil || h.Subjects == nil || h.Service == nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	users, err := h.Subjects.Users(r.Context(), [][]byte{sub})
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	userID := users[base64.RawURLEncoding.EncodeToString(sub)]
	if userID <= 0 {
		writeJSON(w, http.StatusOK, map[string]string{"status": "unknown_user"})
		return
	}
	res, err := h.Service.Reply(r.Context(), userID, body.Text, "d:"+body.EventID)
	if errors.Is(err, ErrTimeout) {
		writeErr(w, http.StatusGatewayTimeout, "timeout")
		return
	}
	if errors.Is(err, ErrBusy) {
		writeErr(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, "model")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func eventID(id string) bool {
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

func decodeSub(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "=") {
		return nil, errors.New("sub")
	}
	buf, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(buf) != 16 {
		return nil, errors.New("sub")
	}
	return buf, nil
}

func authorized(r *http.Request, key []byte) bool {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
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
