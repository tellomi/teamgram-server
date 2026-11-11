package report

import (
	"context"
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
	pathPending         = "/internal/v1/reports/pending"
	pathIngest          = "/internal/v1/reports"
	pathSubjects        = "/internal/v1/reports/subjects"
	pathSubjectsByUsers = "/internal/v1/reports/subjects/by-users"
	pathModeration      = "/internal/v1/moderation/review"
	maxBody             = 64 << 10
	// maxReportSubs is one report's reporter and author, with room for a
	// short batch. content-engine sends the subs it still cannot map.
	maxReportSubs = 32
)

// subjectRetryAfter is the Retry-After delay, in seconds, for a dependency
// failure of this listener. The JSON field retry_after carries the same
// number. Callers must not cache any id from that response.
const subjectRetryAfter = 5

const (
	// subjectMapped is a live account with one 16-byte discover_subject row.
	subjectMapped = "mapped"
	// subjectEmpty is a live account whose discover_subject read succeeded
	// and found no row: an authorized empty discover identity.
	subjectEmpty = "empty"
	// subjectNotFound means users has no such id.
	subjectNotFound = "not_found"
	// subjectClosed means users.deleted or users.restricted is set.
	subjectClosed = "closed"
)

// subjectHit is one classified users.id from POST .../subjects/by-users.
// Sub is set only when Status is subjectMapped, and then it is 16 bytes.
type subjectHit struct {
	UserID int64
	Status string
	Sub    []byte
}

// Subjects resolves content-engine subs through discover_subject.
// The map key of Users is the canonical base64url (no padding) of the
// 16-byte sub. Subs that are not in the table are omitted.
type Subjects interface {
	Users(ctx context.Context, subs [][]byte) (map[string]int64, error)
	// Subs classifies every requested id, in the same order, with no gaps.
	// A non-nil error means the read did not finish. An empty or partial
	// slice is not an authoritative answer.
	Subs(ctx context.Context, userIDs []int64) ([]subjectHit, error)
}

// Handler is the internal report listener. It is mounted only when
// TELLOMI_REPORT_HTTP=1.
//
//	POST /internal/v1/reports
//	POST /internal/v1/reports/subjects
//	POST /internal/v1/reports/subjects/by-users
//	POST /internal/v1/reports/{id}/decision
//	GET  /internal/v1/reports/pending
//	POST /internal/v1/moderation/review
//
// Discover push stays on POST /internal/v1/discover/notify
// (docs/api/discover-ai.md). @tellomi mentions stay on
// POST /internal/v1/ai/complete (docs/api/tellomi-ai.md).
type Handler struct {
	Service *Service
	Key     []byte
	// Subjects overrides the discover_subject lookup. Nil uses the MySQL
	// store on Service when that store is connected.
	Subjects Subjects
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
	switch r.URL.Path {
	case pathPending:
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "method")
			return
		}
		h.pending(w, r)
		return
	case pathIngest:
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "method")
			return
		}
		h.ingest(w, r)
		return
	case pathSubjects:
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "method")
			return
		}
		h.subjects(w, r)
		return
	case pathSubjectsByUsers:
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "method")
			return
		}
		h.subjectsByUsers(w, r)
		return
	case pathModeration:
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "method")
			return
		}
		h.moderate(w, r)
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

type ingestBody struct {
	ReporterUserID int64  `json:"reporter_user_id"`
	AuthorID       int64  `json:"author_id"`
	PostID         string `json:"post_id"`
	CommentID      string `json:"comment_id"`
	Reason         string `json:"reason"`
	Comment        string `json:"comment"`
	LangCode       string `json:"lang_code"`
}

func (h Handler) ingest(w http.ResponseWriter, r *http.Request) {
	var body ingestBody
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	body.PostID = strings.TrimSpace(body.PostID)
	body.CommentID = strings.TrimSpace(body.CommentID)
	body.Reason = strings.TrimSpace(body.Reason)
	if body.ReporterUserID <= 0 || body.AuthorID <= 0 || !validDiscoverID(body.PostID) || !validDiscoverReason(body.Reason) {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if body.CommentID != "" && !validDiscoverID(body.CommentID) {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if len(body.Comment) > 1024 {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	row, dup, err := h.Service.IngestDiscover(r.Context(), DiscoverReport{
		Reporter:  body.ReporterUserID,
		Author:    body.AuthorID,
		PostID:    body.PostID,
		CommentID: body.CommentID,
		Reason:    body.Reason,
		Comment:   body.Comment,
		Lang:      body.LangCode,
	})
	if wait, ok := floodWait(err); ok {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate_limited", "retry_after": wait})
		return
	}
	if err != nil || row == nil {
		writeErr(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	status := "created"
	if dup {
		status = "duplicate"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": status,
		"id":     row.ID,
		"ticket": row.Ticket(),
	})
}

type subjectUser struct {
	Sub    string `json:"sub"`
	UserID int64  `json:"user_id"`
}

type subjectsBody struct {
	Subs []string `json:"subs"`
}

func (h Handler) subjects(w http.ResponseWriter, r *http.Request) {
	var body subjectsBody
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if len(body.Subs) == 0 || len(body.Subs) > maxReportSubs {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	raws := make([][]byte, 0, len(body.Subs))
	order := make([]string, 0, len(body.Subs))
	seen := map[string]struct{}{}
	for _, item := range body.Subs {
		sub, err := decodeReportSub(item)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request")
			return
		}
		key := base64.RawURLEncoding.EncodeToString(sub)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		raws = append(raws, sub)
		order = append(order, key)
	}
	dir := h.subjectDir()
	if dir == nil {
		writeErr(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	found, err := dir.Users(r.Context(), raws)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	users := make([]subjectUser, 0)
	for _, key := range order {
		id := found[key]
		if id <= 0 {
			continue
		}
		users = append(users, subjectUser{Sub: key, UserID: id})
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

type subjectsByUsersBody struct {
	UserIDs []int64 `json:"user_ids"`
}

func (h Handler) subjectsByUsers(w http.ResponseWriter, r *http.Request) {
	var body subjectsByUsersBody
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if len(body.UserIDs) == 0 || len(body.UserIDs) > maxReportSubs {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	ids := make([]int64, 0, len(body.UserIDs))
	seen := map[int64]struct{}{}
	for _, id := range body.UserIDs {
		if id <= 0 {
			writeErr(w, http.StatusBadRequest, "bad_request")
			return
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	dir := h.subjectDir()
	if dir == nil {
		writeErr(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	found, err := dir.Subs(r.Context(), ids)
	if err != nil || !subjectResultsOK(ids, found) {
		writeErr(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	users := make([]subjectByUser, 0, len(found))
	for _, hit := range found {
		item := subjectByUser{UserID: hit.UserID, Status: hit.Status}
		if hit.Status == subjectMapped {
			item.Sub = base64.RawURLEncoding.EncodeToString(hit.Sub)
		}
		users = append(users, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

// subjectByUser is one by-users result. Status is always set. Sub is set
// only for subjectMapped; omitempty keeps it off the other three statuses.
type subjectByUser struct {
	UserID int64  `json:"user_id"`
	Status string `json:"status"`
	Sub    string `json:"sub,omitempty"`
}

func subjectResultsOK(ids []int64, got []subjectHit) bool {
	if len(got) != len(ids) {
		return false
	}
	for i, id := range ids {
		hit := got[i]
		if hit.UserID != id {
			return false
		}
		switch hit.Status {
		case subjectMapped:
			if len(hit.Sub) != 16 {
				return false
			}
		case subjectEmpty, subjectNotFound, subjectClosed:
			if len(hit.Sub) != 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (h Handler) subjectDir() Subjects {
	if h.Subjects != nil {
		return h.Subjects
	}
	if h.Service == nil {
		return nil
	}
	idx, ok := h.Service.store.(*mysqlStore)
	if !ok || idx == nil {
		return nil
	}
	return idx
}

func decodeReportSub(raw string) ([]byte, error) {
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

type moderateBody struct {
	PostID    string   `json:"post_id"`
	Text      string   `json:"text"`
	MediaKeys []string `json:"media_keys"`
}

func (h Handler) moderate(w http.ResponseWriter, r *http.Request) {
	var body moderateBody
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	text := strings.TrimSpace(body.Text)
	var media []string
	for _, key := range body.MediaKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if !validMediaKey(key) {
			writeErr(w, http.StatusBadRequest, "bad_request")
			return
		}
		media = append(media, key)
	}
	if text == "" && len(media) == 0 {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if body.PostID != "" && !validDiscoverID(strings.TrimSpace(body.PostID)) {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	verdict := h.Service.Moderate(r.Context(), text, media)
	writeJSON(w, http.StatusOK, map[string]string{"verdict": verdict})
}

func floodWait(err error) (int32, bool) {
	if err == nil {
		return 0, false
	}
	const marker = "FLOOD_WAIT_"
	msg := err.Error()
	i := strings.Index(msg, marker)
	if i < 0 {
		return 0, false
	}
	n := 0
	for _, c := range msg[i+len(marker):] {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	if n < 1 {
		n = 1
	}
	return int32(n), true
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
	if status == http.StatusServiceUnavailable {
		w.Header().Set("Retry-After", strconv.Itoa(subjectRetryAfter))
		writeJSON(w, status, map[string]any{"error": code, "retry_after": subjectRetryAfter})
		return
	}
	writeJSON(w, status, map[string]string{"error": code})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
