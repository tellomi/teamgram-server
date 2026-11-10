package crashreport

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type lister interface {
	List(context.Context, Query) ([]Row, error)
}

const queryPath = "/internal/v1/crash-reports"

// Handler is GET /internal/v1/crash-reports.
type Handler struct {
	Service lister
	Token   string
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != queryPath {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method")
		return
	}
	if !tokenOK(r, h.Token) {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.Service == nil {
		writeErr(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	q, err := parseQuery(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	rows, err := h.Service.List(r.Context(), q)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "error")
		return
	}
	if rows == nil {
		rows = []Row{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Reports []Row `json:"reports"`
	}{Reports: rows})
}

func parseQuery(r *http.Request) (Query, error) {
	var q Query
	vals := r.URL.Query()
	var err error
	if q.From, err = parseTime(vals.Get("from")); err != nil {
		return q, err
	}
	if q.To, err = parseTime(vals.Get("to")); err != nil {
		return q, err
	}
	if raw := strings.TrimSpace(vals.Get("sender")); raw != "" {
		q.Sender, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || q.Sender < 0 {
			return q, errBad
		}
	}
	q.Device = strings.TrimSpace(vals.Get("device"))
	if raw := strings.TrimSpace(vals.Get("limit")); raw != "" {
		n, conv := strconv.Atoi(raw)
		if conv != nil || n < 1 || n > 200 {
			return q, errBad
		}
		q.Limit = n
	}
	if !q.From.IsZero() && !q.To.IsZero() && q.To.Before(q.From) {
		return q, errBad
	}
	return q, nil
}

func parseTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if n < 0 {
			return time.Time{}, errBad
		}
		return time.Unix(n, 0).UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func tokenOK(r *http.Request, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	got := strings.TrimSpace(r.Header.Get("X-Admin-Token"))
	if got == "" {
		auth := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if len(auth) >= len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
			got = strings.TrimSpace(auth[len(prefix):])
		}
	}
	if len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// AddrAllowed reports whether addr is safe to bind: loopback, a private
// address, or a host name that resolves only to those. 0.0.0.0 is refused.
func AddrAllowed(addr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil || host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}`))
}

var errBad = errText("bad request")

type errText string

func (e errText) Error() string { return string(e) }
