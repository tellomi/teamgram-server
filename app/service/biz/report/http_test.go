package report

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestDecisionEndpoint(t *testing.T) {
	s, box, _ := testService(t)
	s.SetLangs(mapLang{20: "zh-hans"})
	ctx := context.Background()
	row, err := s.Submit(ctx, Submission{
		Source: SourcePeer, Reporter: 424242, Lang: "en",
		PeerType: mtproto.PEER_USER, PeerID: 20, SubjectUserID: 20, Reason: "spam",
	})
	if err != nil {
		t.Fatal(err)
	}
	key := bytesKey(t)
	h := Handler{Service: s, Key: key}
	token := base64.StdEncoding.EncodeToString(key)

	req := httptest.NewRequest(http.MethodGet, "/internal/v1/reports/pending", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("auth %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/internal/v1/reports/pending", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ticket":"T`) {
		t.Fatalf("pending %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/internal/v1/reports/"+itoa(row.ID)+"/decision", strings.NewReader(`{"decision":"violation"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("decide %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err = json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["status"] != StatusViolation {
		t.Fatalf("%v %v", body, err)
	}
	if len(box.to(20)) != 1 || strings.Contains(box.to(20)[0], "424242") || !strings.Contains(box.to(20)[0], "经平台审核") {
		t.Fatalf("subject %+v", box.to(20))
	}
	pending, err := s.Pending(ctx, 10)
	if err != nil || len(pending) != 0 {
		t.Fatalf("queue left %+v %v", pending, err)
	}

	req = httptest.NewRequest(http.MethodPost, "/internal/v1/reports/"+itoa(row.ID)+"/decision", strings.NewReader(`{"decision":"clear"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("conflict %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/internal/v1/reports/99/decision", strings.NewReader(`{"decision":"no_violation"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing %d", rec.Code)
	}
}

func bytesKey(t *testing.T) []byte {
	t.Helper()
	return []byte("0123456789abcdef0123456789abcdef")
}

func TestDiscoverIngestDedupesAndReceipt(t *testing.T) {
	s, box, _ := testService(t)
	key := bytesKey(t)
	h := Handler{Service: s, Key: key}
	token := base64.StdEncoding.EncodeToString(key)
	body := `{"reporter_user_id":424242,"author_id":909090,"post_id":"post-9","reason":"spam","comment":"bad","lang_code":"zh-hans"}`

	rec := postJSON(t, h, token, "/internal/v1/reports", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created["status"] != "created" || created["ticket"] != "T1" {
		t.Fatalf("%v %v", created, err)
	}
	rows, err := s.List(context.Background())
	if err != nil || len(rows) != 1 || rows[0].Source != SourcePost || rows[0].SubjectUserID != 909090 || rows[0].Reason != "spam" || rows[0].Message != "bad" {
		t.Fatalf("%+v %v", rows, err)
	}
	if rows[0].TargetKey != "post:post-9" {
		t.Fatalf("target %s", rows[0].TargetKey)
	}
	notes := box.to(424242)
	if len(notes) != 1 || !strings.Contains(notes[0], "T1") || !strings.Contains(notes[0], "经平台审核") || strings.Contains(strings.ToLower(notes[0]), "ai") {
		t.Fatalf("receipt %+v", notes)
	}

	rec = postJSON(t, h, token, "/internal/v1/reports", body)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"duplicate"`) {
		t.Fatalf("dup %d %s", rec.Code, rec.Body.String())
	}
	rows, err = s.List(context.Background())
	if err != nil || len(rows) != 1 || len(box.to(424242)) != 1 {
		t.Fatalf("rows %d receipts %d %v", len(rows), len(box.to(424242)), err)
	}

	other := `{"reporter_user_id":515151,"author_id":909090,"post_id":"post-9","comment_id":"c-1","reason":"other","comment":"x","lang_code":"en"}`
	rec = postJSON(t, h, token, "/internal/v1/reports", other)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"created"`) {
		t.Fatalf("comment %d %s", rec.Code, rec.Body.String())
	}
	rows, _ = s.List(context.Background())
	if len(rows) != 2 || rows[1].Source != "discover.comment" || rows[1].TargetKey != "comment:post-9:c-1" {
		t.Fatalf("%+v", rows)
	}

	rec = postJSON(t, h, "", "/internal/v1/reports", body)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("auth %d", rec.Code)
	}
	rec = postJSON(t, h, token, "/internal/v1/reports", `{"reporter_user_id":1,"author_id":2,"post_id":"post-1","reason":"nope"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("reason %d", rec.Code)
	}
}

func TestDiscoverIngestRateLimit(t *testing.T) {
	s, _, _ := testService(t)
	s.limit = 1
	h := Handler{Service: s, Key: bytesKey(t)}
	token := base64.StdEncoding.EncodeToString(h.Key)
	first := `{"reporter_user_id":7,"author_id":8,"post_id":"p1","reason":"violence"}`
	if rec := postJSON(t, h, token, "/internal/v1/reports", first); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	second := `{"reporter_user_id":7,"author_id":8,"post_id":"p2","reason":"drugs"}`
	rec := postJSON(t, h, token, "/internal/v1/reports", second)
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), `"rate_limited"`) {
		t.Fatalf("limit %d %s", rec.Code, rec.Body.String())
	}
}

func TestModerationVerdicts(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		reply string
		want  string
	}{
		{`{"result":"violation"}`, "violation"},
		{`{"result":"clear"}`, "ok"},
		{`{"result":"needs_human"}`, "uncertain"},
	} {
		s, _, _ := testService(t)
		llm := &fakeLLM{reply: tc.reply}
		s.SetReviewer(NewReviewer(llm))
		h := Handler{Service: s, Key: bytesKey(t)}
		token := base64.StdEncoding.EncodeToString(h.Key)
		rec := postJSON(t, h, token, "/internal/v1/moderation/review", `{"post_id":"post-3","text":"keyword hit","media_keys":["photos/9","videos/2"]}`)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"verdict":"`+tc.want+`"`) {
			t.Fatalf("%s -> %d %s", tc.reply, rec.Code, rec.Body.String())
		}
		if llm.calls != 1 {
			t.Fatalf("calls %d", llm.calls)
		}
		rows, err := s.List(ctx)
		if err != nil || len(rows) != 0 {
			t.Fatalf("moderation stored a report %+v %v", rows, err)
		}
	}

	s, _, _ := testService(t)
	llm := &fakeLLM{reply: `{"result":"violation"}`}
	h := Handler{Service: s, Key: bytesKey(t)}
	token := base64.StdEncoding.EncodeToString(h.Key)
	rec := postJSON(t, h, token, "/internal/v1/moderation/review", `{"text":"","media_keys":["photos/1"]}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"uncertain"`) || llm.calls != 0 {
		t.Fatalf("off %d %s calls %d", rec.Code, rec.Body.String(), llm.calls)
	}
	rec = postJSON(t, h, token, "/internal/v1/moderation/review", `{"text":"","media_keys":[]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty %d", rec.Code)
	}
	rec = postJSON(t, h, token, "/internal/v1/moderation/review", `{"text":"x","media_keys":["../secret"]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("media %d", rec.Code)
	}
}

type mapSubjects struct {
	users map[string]int64
	err   error
}

func (m mapSubjects) Users(_ context.Context, subs [][]byte) (map[string]int64, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := map[string]int64{}
	for _, sub := range subs {
		key := base64.RawURLEncoding.EncodeToString(sub)
		if id := m.users[key]; id > 0 {
			out[key] = id
		}
	}
	return out, nil
}

func (m mapSubjects) Subs(_ context.Context, userIDs []int64) ([]subjectHit, error) {
	if m.err != nil {
		return nil, m.err
	}
	inv := map[int64][]byte{}
	for key, id := range m.users {
		if id <= 0 {
			continue
		}
		raw, err := base64.RawURLEncoding.DecodeString(key)
		if err != nil || len(raw) != 16 {
			continue
		}
		inv[id] = raw
	}
	out := make([]subjectHit, 0, len(userIDs))
	for _, id := range userIDs {
		if sub := inv[id]; len(sub) == 16 {
			out = append(out, subjectHit{UserID: id, Status: subjectMapped, Sub: sub})
			continue
		}
		// This double has no users table. An id it cannot map is not_found,
		// which is distinct from an authorized empty homepage.
		out = append(out, subjectHit{UserID: id, Status: subjectNotFound})
	}
	return out, nil
}

type statusSubjects struct {
	hits []subjectHit
	err  error
}

func (s statusSubjects) Users(context.Context, [][]byte) (map[string]int64, error) {
	return map[string]int64{}, nil
}

func (s statusSubjects) Subs(context.Context, []int64) ([]subjectHit, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.hits, nil
}

func TestReportSubjects(t *testing.T) {
	s, box, _ := testService(t)
	known := bytes.Repeat([]byte{7}, 16)
	unknown := bytes.Repeat([]byte{8}, 16)
	knownEnc := base64.RawURLEncoding.EncodeToString(known)
	unknownEnc := base64.RawURLEncoding.EncodeToString(unknown)
	h := Handler{
		Service:  s,
		Key:      bytesKey(t),
		Subjects: mapSubjects{users: map[string]int64{knownEnc: 34}},
	}
	token := base64.StdEncoding.EncodeToString(h.Key)

	rec := postJSON(t, h, "", pathSubjects, `{"subs":["`+knownEnc+`"]}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("auth %d %s", rec.Code, rec.Body.String())
	}

	body := `{"subs":["` + knownEnc + `","` + knownEnc + `","` + unknownEnc + `"]}`
	rec = postJSON(t, h, token, pathSubjects, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("lookup %d %s", rec.Code, rec.Body.String())
	}
	var parsed struct {
		Users []subjectUser `json:"users"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Users) != 1 || parsed.Users[0].Sub != knownEnc || parsed.Users[0].UserID != 34 {
		t.Fatalf("%+v", parsed.Users)
	}
	rows, err := s.List(context.Background())
	if err != nil || len(rows) != 0 || len(box.msgs) != 0 {
		t.Fatalf("stored rows %d receipts %d err %v", len(rows), len(box.msgs), err)
	}

	rec = postJSON(t, h, token, pathSubjects, `{"subs":["`+unknownEnc+`"]}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"users":[]`) {
		t.Fatalf("unknown %d %s", rec.Code, rec.Body.String())
	}
	rec = postJSON(t, h, token, pathSubjects, `{"subs":[]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty %d", rec.Code)
	}
	rec = postJSON(t, h, token, pathSubjects, `{"subs":["abc"]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("shape %d", rec.Code)
	}
	rec = postJSON(t, h, token, pathSubjects, `{"subs":["`+knownEnc+`="]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("padding %d", rec.Code)
	}
	tooMany := `{"subs":[` + strings.TrimRight(strings.Repeat(`"`+knownEnc+`",`, maxReportSubs+1), ",") + `]}`
	rec = postJSON(t, h, token, pathSubjects, tooMany)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("cap %d %s", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, pathSubjects, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method %d", rec.Code)
	}

	h.Subjects = mapSubjects{err: errors.New("down")}
	rec = postJSON(t, h, token, pathSubjects, `{"subs":["`+knownEnc+`"]}`)
	assertUnavailable(t, rec)
	h.Subjects = nil
	rec = postJSON(t, h, token, pathSubjects, `{"subs":["`+knownEnc+`"]}`)
	assertUnavailable(t, rec)
}

func TestReportSubjectsByUsers(t *testing.T) {
	s, box, _ := testService(t)
	known := bytes.Repeat([]byte{7}, 16)
	knownEnc := base64.RawURLEncoding.EncodeToString(known)
	h := Handler{
		Service:  s,
		Key:      bytesKey(t),
		Subjects: mapSubjects{users: map[string]int64{knownEnc: 34}},
	}
	token := base64.StdEncoding.EncodeToString(h.Key)

	rec := postJSON(t, h, "", pathSubjectsByUsers, `{"user_ids":[34]}`)
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("Retry-After") != "" {
		t.Fatalf("auth %d %q %s", rec.Code, rec.Header().Get("Retry-After"), rec.Body.String())
	}

	rec = postJSON(t, h, token, pathSubjectsByUsers, `{"user_ids":[34,34,99]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("lookup %d %s", rec.Code, rec.Body.String())
	}
	parsed := decodeByUsers(t, rec.Body.Bytes())
	if len(parsed) != 2 || parsed[0].Status != subjectMapped || parsed[0].Sub != knownEnc || parsed[0].UserID != 34 || parsed[1].Status != subjectNotFound || parsed[1].UserID != 99 || parsed[1].Sub != "" {
		t.Fatalf("%+v", parsed)
	}
	if strings.Contains(rec.Body.String(), `"user_id":99,"status":"not_found","sub"`) {
		t.Fatalf("not_found leaked a sub: %s", rec.Body.String())
	}
	rows, err := s.List(context.Background())
	if err != nil || len(rows) != 0 || len(box.msgs) != 0 {
		t.Fatalf("stored rows %d receipts %d err %v", len(rows), len(box.msgs), err)
	}

	rec = postJSON(t, h, token, pathSubjectsByUsers, `{"user_ids":[99]}`)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"users":[]`) {
		t.Fatalf("unknown %d %s", rec.Code, rec.Body.String())
	}
	parsed = decodeByUsers(t, rec.Body.Bytes())
	if len(parsed) != 1 || parsed[0].Status != subjectNotFound || parsed[0].UserID != 99 {
		t.Fatalf("unknown %+v", parsed)
	}
	rec = postJSON(t, h, token, pathSubjectsByUsers, `{"user_ids":[]}`)
	if rec.Code != http.StatusBadRequest || rec.Header().Get("Retry-After") != "" {
		t.Fatalf("empty %d %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	rec = postJSON(t, h, token, pathSubjectsByUsers, `{"user_ids":[0]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("zero %d", rec.Code)
	}
	rec = postJSON(t, h, token, pathSubjectsByUsers, `{"user_ids":[-1]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("neg %d", rec.Code)
	}
	rec = postJSON(t, h, token, pathSubjectsByUsers, `{"user_ids":[34]}{"user_ids":[35]}`)
	if rec.Code != http.StatusBadRequest || rec.Header().Get("Retry-After") != "" {
		t.Fatalf("trailing %d %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	rec = postJSON(t, h, token, pathSubjectsByUsers, `{"user_ids":[34],"user_ids":[35]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("duplicate key %d %s", rec.Code, rec.Body.String())
	}
	rec = postJSON(t, h, token, pathSubjectsByUsers, `{"user_ids":[34],"extra":true}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field %d %s", rec.Code, rec.Body.String())
	}
	tooMany := `{"user_ids":[` + strings.TrimRight(strings.Repeat(`34,`, maxReportSubs+1), ",") + `]}`
	rec = postJSON(t, h, token, pathSubjectsByUsers, tooMany)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("cap %d %s", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, pathSubjectsByUsers, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Retry-After") != "" {
		t.Fatalf("method %d %q", rec.Code, rec.Header().Get("Retry-After"))
	}

	h.Subjects = mapSubjects{err: errors.New("down")}
	rec = postJSON(t, h, token, pathSubjectsByUsers, `{"user_ids":[34]}`)
	assertUnavailable(t, rec)
	h.Subjects = nil
	rec = postJSON(t, h, token, pathSubjectsByUsers, `{"user_ids":[34]}`)
	assertUnavailable(t, rec)
	if rec.Header().Get("Authorization") != "" {
		t.Fatal("auth header echoed")
	}
}

func TestReview3StrictByUsersRequest(t *testing.T) {
	s, _, _ := testService(t)
	h := Handler{
		Service:  s,
		Key:      bytesKey(t),
		Subjects: mapSubjects{users: map[string]int64{}},
	}
	token := base64.StdEncoding.EncodeToString(h.Key)
	for _, body := range []string{
		`{"user_ids":[34]}]`,
		`{"user_ids":[34]}}`,
		`{"user_ids":[34]}]{"user_ids":[35]}`,
		`{"user_ids":[34],"USER_IDS":[35]}`,
		`{"USER_IDS":[34]}`,
	} {
		t.Run(body, func(t *testing.T) {
			if _, err := decodeByUsersBody(strings.NewReader(body)); err == nil {
				t.Fatal("decoder accepted")
			}
			rec := postJSON(t, h, token, pathSubjectsByUsers, body)
			if rec.Code != http.StatusBadRequest || rec.Header().Get("Retry-After") != "" {
				t.Fatalf("%d %q %s", rec.Code, rec.Header().Get("Retry-After"), rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), `"users"`) || strings.Contains(rec.Body.String(), `"user_id"`) {
				t.Fatalf("handler used an id: %s", rec.Body.String())
			}
		})
	}
}

func TestSubjectsByUsersContract(t *testing.T) {
	s, _, _ := testService(t)
	known := bytes.Repeat([]byte{7}, 16)
	knownEnc := base64.RawURLEncoding.EncodeToString(known)
	h := Handler{Service: s, Key: bytesKey(t)}
	token := base64.StdEncoding.EncodeToString(h.Key)

	h.Subjects = statusSubjects{hits: []subjectHit{
		{UserID: 36, Status: subjectNotFound},
		{UserID: 34, Status: subjectMapped, Sub: known},
		{UserID: 35, Status: subjectEmpty},
		{UserID: 37, Status: subjectClosed},
	}}
	rec := postJSON(t, h, token, pathSubjectsByUsers, `{"user_ids":[36,34,35,37]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("contract %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") != "" {
		t.Fatal("success carried Retry-After")
	}
	parsed := decodeByUsers(t, rec.Body.Bytes())
	want := []subjectByUser{
		{UserID: 36, Status: subjectNotFound},
		{UserID: 34, Status: subjectMapped, Sub: knownEnc},
		{UserID: 35, Status: subjectEmpty},
		{UserID: 37, Status: subjectClosed},
	}
	if len(parsed) != len(want) {
		t.Fatalf("%+v", parsed)
	}
	for i := range want {
		if parsed[i] != want[i] {
			t.Fatalf("item %d %+v", i, parsed)
		}
	}
	var raw struct {
		Users []map[string]any `json:"users"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, item := range raw.Users {
		status, _ := item["status"].(string)
		_, hasSub := item["sub"]
		if status == subjectMapped && !hasSub {
			t.Fatalf("mapped without sub %+v", item)
		}
		if status != subjectMapped && hasSub {
			t.Fatalf("%s leaked sub %+v", status, item)
		}
	}

	// Omitting an id, an unknown status, a short sub, or a sub on a closed
	// account must not become a 200 that content-engine can cache.
	h.Subjects = statusSubjects{hits: []subjectHit{{UserID: 34, Status: subjectMapped, Sub: known}}}
	rec = postJSON(t, h, token, pathSubjectsByUsers, `{"user_ids":[34,99]}`)
	assertUnavailable(t, rec)

	h.Subjects = statusSubjects{hits: nil}
	rec = postJSON(t, h, token, pathSubjectsByUsers, `{"user_ids":[34]}`)
	assertUnavailable(t, rec)

	h.Subjects = statusSubjects{hits: []subjectHit{{UserID: 34, Status: "absent"}}}
	rec = postJSON(t, h, token, pathSubjectsByUsers, `{"user_ids":[34]}`)
	assertUnavailable(t, rec)

	h.Subjects = statusSubjects{hits: []subjectHit{{UserID: 34, Status: subjectMapped, Sub: []byte{1, 2, 3}}}}
	rec = postJSON(t, h, token, pathSubjectsByUsers, `{"user_ids":[34]}`)
	assertUnavailable(t, rec)

	h.Subjects = statusSubjects{hits: []subjectHit{{UserID: 37, Status: subjectClosed, Sub: known}}}
	rec = postJSON(t, h, token, pathSubjectsByUsers, `{"user_ids":[37]}`)
	assertUnavailable(t, rec)

	h.Subjects = statusSubjects{err: errSubjectsUnavailable}
	rec = postJSON(t, h, token, pathSubjectsByUsers, `{"user_ids":[34]}`)
	assertUnavailable(t, rec)
	h.Subjects = statusSubjects{err: errSubjectCorrupt}
	rec = postJSON(t, h, token, pathSubjectsByUsers, `{"user_ids":[34]}`)
	assertUnavailable(t, rec)
}

func decodeByUsers(t *testing.T, raw []byte) []subjectByUser {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var parsed struct {
		Users []subjectByUser `json:"users"`
	}
	if err := dec.Decode(&parsed); err != nil {
		t.Fatalf("decode %v %s", err, raw)
	}
	return parsed.Users
}

func assertUnavailable(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Retry-After"); got != strconv.Itoa(subjectRetryAfter) {
		t.Fatalf("Retry-After %q", got)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache %q", rec.Header().Get("Cache-Control"))
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "unavailable" || body["retry_after"] != float64(subjectRetryAfter) {
		t.Fatalf("body %+v", body)
	}
	if _, ok := body["users"]; ok {
		t.Fatalf("unavailable leaked users %+v", body)
	}
}

func postJSON(t *testing.T, h Handler, token, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
