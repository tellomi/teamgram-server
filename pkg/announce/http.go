package announce

import (
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type body struct {
	EventID string `json:"event_id"`
	Text    string `json:"text"`
	Cursor  string `json:"cursor"`
}

// Handler is POST /internal/v1/announce.
type Handler struct {
	Service *Service
	Key     []byte
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != pathPost {
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
	var in body
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	after, err := validate(strings.TrimSpace(in.EventID), strings.TrimSpace(in.Text), in.Cursor)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if h.Service == nil {
		writeErr(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	out, err := h.Service.Deliver(r.Context(), Request{
		EventID: strings.TrimSpace(in.EventID),
		Text:    strings.TrimSpace(in.Text),
		After:   after,
	})
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  out.Status,
		"cursor":  strconv.FormatInt(out.Cursor, 10),
		"sent":    out.Sent,
		"channel": out.Channel,
	})
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
