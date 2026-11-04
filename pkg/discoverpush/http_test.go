package discoverpush

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/teamgram/teamgram-server/pkg/push"
)

type capFCM struct {
	n    int
	body []byte
}

func (c *capFCM) Send(_ context.Context, _ string, payload []byte) error {
	c.n++
	c.body = append([]byte(nil), payload...)
	return nil
}

type capHuawei struct {
	n    int
	data string
}

func (c *capHuawei) Send(_ context.Context, _ string, data string) error {
	c.n++
	c.data = data
	return nil
}

type settings struct {
	*push.MemoryStore
	muted bool
}

func (s *settings) NotifySettings(_ context.Context, _ int64, _ int32, peerID int64) (push.NotifySettings, error) {
	if s.muted && peerID == push.DiscoverNotifyUserID {
		return push.NotifySettings{Found: true, ShowPreviews: 1, MuteUntil: int32(time.Now().Unix()) + 3600}, nil
	}
	return push.NotifySettings{Found: true, ShowPreviews: 1}, nil
}

func TestNotifyE2E(t *testing.T) {
	secret := bytes.Repeat([]byte{0x11}, 256)
	store := &settings{MemoryStore: &push.MemoryStore{}}
	ctx := context.Background()
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 1, UserID: 42, TokenType: push.TokenFCM, Token: "fcm", Secret: secret, LangCode: "zh-Hans"})
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 2, UserID: 42, TokenType: push.TokenHuawei, Token: "hms", Secret: secret, LangCode: "zh-Hans"})
	_ = store.Upsert(ctx, push.Device{AuthKeyID: 3, UserID: 42, TokenType: push.TokenAPNS, Token: "apns", Secret: secret, LangCode: "zh-Hans"})
	fcm := &capFCM{}
	hms := &capHuawei{}
	svc := push.NewService(store, push.Config{})
	svc.SetFCM(fcm)
	svc.SetHuawei(hms)

	sub := bytes.Repeat([]byte{9}, 16)
	subjects := NewMemorySubjects()
	subjects.Save(42, sub)
	key := bytes.Repeat([]byte{7}, 32)
	h := Handler{Subjects: subjects, Events: NewMemoryEvents(), Push: svc, Key: key}
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := map[string]string{
		"event_id":   "ntf-1",
		"recipient":  base64.RawURLEncoding.EncodeToString(sub),
		"kind":       "comment",
		"actor_name": "林",
		"post_id":    "post-9",
		"text":       "好看",
	}
	status := postNotify(t, srv.URL, base64.StdEncoding.EncodeToString(key), body)
	if status != "sent" {
		t.Fatalf("status %s", status)
	}
	if fcm.n != 1 || hms.n != 1 {
		t.Fatalf("fcm %d huawei %d", fcm.n, hms.n)
	}
	if !bytes.Contains(fcm.body, []byte("DISCOVER_COMMENT_TEXT")) || !bytes.Contains(fcm.body, []byte("好看")) || !bytes.Contains(fcm.body, []byte("post-9")) {
		t.Fatalf("fcm %s", fcm.body)
	}
	plain, err := push.DecryptNotification(secret, hms.data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(plain, []byte("DISCOVER_COMMENT_TEXT")) || !bytes.Contains(plain, []byte(`"discover":"1"`)) {
		t.Fatalf("huawei %s", plain)
	}

	if again := postNotify(t, srv.URL, base64.StdEncoding.EncodeToString(key), body); again != "duplicate" || fcm.n != 1 {
		t.Fatalf("duplicate status %s fcm %d", again, fcm.n)
	}

	if postNotify(t, srv.URL+"/nope", base64.StdEncoding.EncodeToString(key), body) != "" {
		t.Fatal("wrong path should not be a status")
	}
	wrong := httptest.NewRequest(http.MethodPost, pathNotify, bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wrong)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("auth %d", rec.Code)
	}

	body["event_id"] = "ntf-unknown"
	body["recipient"] = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{3}, 16))
	if postNotify(t, srv.URL, base64.StdEncoding.EncodeToString(key), body) != "unknown_recipient" {
		t.Fatal("unknown recipient")
	}

	body["event_id"] = "ntf-bad"
	body["recipient"] = base64.RawURLEncoding.EncodeToString(sub)
	body["kind"] = "reply"
	body["comment_id"] = ""
	rec = httptest.NewRecorder()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, pathNotify, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+base64.StdEncoding.EncodeToString(key))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("reply without comment %d %s", rec.Code, rec.Body.Bytes())
	}

	store.muted = true
	body["kind"] = "like"
	body["event_id"] = "ntf-mute"
	body["comment_id"] = ""
	if postNotify(t, srv.URL, base64.StdEncoding.EncodeToString(key), body) != "muted" || fcm.n != 1 {
		t.Fatalf("mute did not hold, fcm %d", fcm.n)
	}
}

func TestStartDefaultOff(t *testing.T) {
	t.Setenv(EnvEnabled, "")
	Start(nil, "")
}

func postNotify(t *testing.T, url, key string, body map[string]string) string {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, url+pathNotify, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return ""
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d %#v", resp.StatusCode, out)
	}
	return out.Status
}
