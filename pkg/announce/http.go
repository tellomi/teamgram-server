package announce

import (
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type body struct {
	EventID  string   `json:"event_id"`
	Text     string   `json:"text"`
	Cursor   string   `json:"cursor"`
	From     int64    `json:"from"`
	DryRun   bool     `json:"dry_run"`
	Markup   string   `json:"markup"`
	Entities []Entity `json:"entities"`
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
	text := strings.TrimSpace(in.Text)
	ents := in.Entities
	switch in.Markup {
	case "":
	case "markdown":
		if len(in.Entities) != 0 {
			writeErr(w, http.StatusBadRequest, "bad_request")
			return
		}
		text, ents = ParseMarkup(text)
	default:
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	after, err := validate(strings.TrimSpace(in.EventID), text, in.Cursor)
	if err != nil || !entitiesInRange(text, ents) {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	from := in.From
	if from == 0 {
		var ok bool
		from, ok = SenderFrom(os.Getenv(EnvFrom))
		if !ok {
			writeErr(w, http.StatusBadRequest, "bad_request")
			return
		}
	} else if !AllowedSender(from) {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if h.Service == nil {
		writeErr(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	req := Request{
		EventID:  strings.TrimSpace(in.EventID),
		Text:     text,
		Entities: ents,
		From:     from,
		After:    after,
	}
	var out Result
	if in.DryRun {
		out, err = h.Service.Preview(r.Context(), req)
	} else {
		out, err = h.Service.Deliver(r.Context(), req)
	}
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	resp := map[string]any{
		"status":  out.Status,
		"cursor":  strconv.FormatInt(out.Cursor, 10),
		"sent":    out.Sent,
		"channel": out.Channel,
		"from":    out.From,
	}
	if out.DryRun {
		resp["dry_run"] = true
		resp["preview"] = out.Preview
		resp["entities"] = out.Entities
		resp["count"] = out.Sent
	}
	writeJSON(w, http.StatusOK, resp)
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
