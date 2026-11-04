package discoverprofile

import (
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

const (
	maxBody  = 64 << 10
	pathPost = "/internal/v1/profiles/resolve"
)

var errBad = errors.New("discoverprofile: bad request")

// Handler is POST /internal/v1/profiles/resolve.
type Handler struct {
	Dir Directory
	Key []byte
}

// ServeHTTP checks the bearer secret and returns privacy-filtered cards.
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
	if h.Dir == nil {
		writeErr(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	var body struct {
		Viewer  string   `json:"viewer"`
		Authors []string `json:"authors"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	if err := dec.Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	viewer, err := decodeSub(body.Viewer)
	if err != nil || len(body.Authors) == 0 || len(body.Authors) > maxAuthors {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	authors := make([][]byte, len(body.Authors))
	seen := map[string]struct{}{}
	order := make([]string, len(body.Authors))
	for i, raw := range body.Authors {
		sub, err := decodeSub(raw)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request")
			return
		}
		authors[i] = sub
		order[i] = subKey(sub)
		seen[order[i]] = struct{}{}
	}
	subs := make([][]byte, 0, len(seen)+1)
	subs = append(subs, viewer)
	for key := range seen {
		if key == subKey(viewer) {
			continue
		}
		raw, _ := base64.RawURLEncoding.DecodeString(key)
		subs = append(subs, raw)
	}
	users, err := h.Dir.Users(r.Context(), subs)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	viewerID := users[subKey(viewer)]
	ids := make([]int64, 0, len(seen))
	for key := range seen {
		if id := users[key]; id > 0 {
			ids = append(ids, id)
		}
	}
	profiles := map[int64]Profile{}
	privacy := map[int64]Privacy{}
	edges := map[int64]Edge{}
	if len(ids) > 0 {
		profiles, err = h.Dir.Profiles(r.Context(), ids)
		if err != nil {
			writeErr(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		privacy, err = h.Dir.Privacy(r.Context(), ids)
		if err != nil {
			writeErr(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
	}
	peers := make([]int64, 0, len(ids))
	for _, id := range ids {
		if viewerID > 0 && id != viewerID {
			peers = append(peers, id)
		}
	}
	if viewerID > 0 && len(peers) > 0 {
		edges, err = h.Dir.Edges(r.Context(), viewerID, peers)
		if err != nil {
			writeErr(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
	}
	writeJSON(w, http.StatusOK, Evaluate(subKey(viewer), viewerID, order, users, profiles, privacy, edges))
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
