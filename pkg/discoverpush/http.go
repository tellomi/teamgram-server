// Package discoverpush accepts content-engine interaction events and hands
// them to pkg/push. The engine keeps the in-app list. This process only
// sends the system push.
package discoverpush

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/teamgram/teamgram-server/pkg/push"
)

const (
	// EnvEnabled turns the listener on. Unset or any other value is off.
	EnvEnabled = "TELLOMI_DISCOVER_PUSH"
	// EnvKey is standard base64 of at least 32 bytes. The engine sends that
	// same base64 as Authorization: Bearer.
	EnvKey = "TELLOMI_DISCOVER_PUSH_KEY"
	// EnvAddr is the listen address. Empty means 127.0.0.1:8813. "off" does not listen.
	EnvAddr = "TELLOMI_DISCOVER_PUSH_ADDR"

	defaultAddr = "127.0.0.1:8813"
	maxBody     = 64 << 10
	pathNotify  = "/internal/v1/discover/notify"
)

var errBad = errors.New("discoverpush: bad request")

// Subjects maps a content-engine sub to a teamgram user id.
type Subjects interface {
	Users(ctx context.Context, subs [][]byte) (map[string]int64, error)
}

// Events claims an engine event id so a retry does not push twice.
type Events interface {
	Claim(ctx context.Context, id string, userID int64) (fresh bool, err error)
	Release(ctx context.Context, id string) error
}

// Handler is POST /internal/v1/discover/notify.
type Handler struct {
	Subjects Subjects
	Events   Events
	Push     *push.Service
	Key      []byte
}

type request struct {
	EventID   string `json:"event_id"`
	Recipient string `json:"recipient"`
	Kind      string `json:"kind"`
	ActorName string `json:"actor_name"`
	PostID    string `json:"post_id"`
	CommentID string `json:"comment_id"`
	Text      string `json:"text"`
}

// ServeHTTP authenticates the engine and pushes one interaction.
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != pathNotify {
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
	var body request
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	if err := dec.Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if err := validate(body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	sub, err := decodeSub(body.Recipient)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if h.Subjects == nil || h.Events == nil || h.Push == nil {
		writeErr(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	users, err := h.Subjects.Users(r.Context(), [][]byte{sub})
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	userID := users[subKey(sub)]
	if userID <= 0 {
		writeJSON(w, http.StatusOK, map[string]string{"status": "unknown_recipient"})
		return
	}
	fresh, err := h.Events.Claim(r.Context(), body.EventID, userID)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	if !fresh {
		writeJSON(w, http.StatusOK, map[string]string{"status": "duplicate"})
		return
	}
	n, err := h.Push.DeliverActivity(r.Context(), push.Activity{
		UserID:    userID,
		Kind:      body.Kind,
		ActorName: body.ActorName,
		PostID:    body.PostID,
		CommentID: body.CommentID,
		Text:      body.Text,
	})
	if err != nil && !errors.Is(err, push.ErrMuted) {
		_ = h.Events.Release(r.Context(), body.EventID)
		writeErr(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	status := "sent"
	switch {
	case errors.Is(err, push.ErrMuted):
		status = "muted"
	case n == 0:
		status = "no_devices"
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

func validate(body request) error {
	if !eventID(body.EventID) {
		return errBad
	}
	switch body.Kind {
	case "like", "comment", "mention", "repost":
		if strings.TrimSpace(body.PostID) == "" {
			return errBad
		}
	case "reply":
		if strings.TrimSpace(body.PostID) == "" || strings.TrimSpace(body.CommentID) == "" {
			return errBad
		}
	case "follow":
	default:
		return errBad
	}
	if utf8.RuneCountInString(body.ActorName) > 80 || utf8.RuneCountInString(body.Text) > 200 {
		return errBad
	}
	if len(body.PostID) > 128 || len(body.CommentID) > 128 {
		return errBad
	}
	return nil
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
		return nil, errBad
	}
	buf, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(buf) != 16 {
		return nil, errBad
	}
	return buf, nil
}

func subKey(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
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
