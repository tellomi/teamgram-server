package report

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
