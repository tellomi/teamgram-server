package announce

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStartOffDoesNotOpen(t *testing.T) {
	t.Setenv(EnvEnabled, "")
	t.Setenv(EnvKey, "")
	Start("not-a-dsn", nil, nil)
}

func TestAdvertisedRequiresKey(t *testing.T) {
	t.Setenv(EnvEnabled, "")
	t.Setenv(EnvKey, "")
	if Advertised() {
		t.Fatal("unset is advertised")
	}
	t.Setenv(EnvEnabled, "1")
	if Advertised() {
		t.Fatal("flag without a key is advertised")
	}
	t.Setenv(EnvKey, base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 31)))
	if Advertised() {
		t.Fatal("short key is advertised")
	}
	t.Setenv(EnvKey, base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)))
	if !Advertised() {
		t.Fatal("flag and key did not advertise")
	}
}

func TestUserPageDoesNotFlush(t *testing.T) {
	if strings.Contains(strings.ToUpper(userPageSQL), "FLUSH") {
		t.Fatal("user page must not flush redis")
	}
	if !strings.Contains(userPageSQL, "NOT BETWEEN 777000 AND 777005") || !strings.Contains(userPageSQL, "deleted = 0") {
		t.Fatalf("query = %s", userPageSQL)
	}
}

type recUsers struct {
	ids  []int64
	fail map[int64]int
}

func (r *recUsers) Send(_ context.Context, id int64, text string) error {
	if text == "" {
		return errors.New("empty")
	}
	if r.fail[id] > 0 {
		r.fail[id]--
		return errors.New("down")
	}
	r.ids = append(r.ids, id)
	return nil
}

type recChannel struct {
	n    int
	fail int
}

func (c *recChannel) Post(context.Context, string) error {
	c.n++
	if c.fail > 0 {
		c.fail--
		return errors.New("down")
	}
	return nil
}

func TestDeliverPagesAndSkipsOfficial(t *testing.T) {
	mem := NewMemory([]int64{3, ServiceUserID, 777001, 9, 10, 11})
	users := &recUsers{}
	ch := &recChannel{}
	svc := &Service{Dir: mem, Claims: mem, Users: users, Channel: ch, PageSize: 2}
	ctx := context.Background()
	first, err := svc.Deliver(ctx, Request{EventID: "launch-1", Text: "上线了", After: 0})
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "partial" || first.Sent != 2 || first.Cursor != 9 || first.Channel != "posted" || ch.n != 1 {
		t.Fatalf("%+v posts %d", first, ch.n)
	}
	if users.ids[0] == ServiceUserID || users.ids[1] == 777001 {
		t.Fatalf("official accounts were mailed: %v", users.ids)
	}
	again, err := svc.Deliver(ctx, Request{EventID: "launch-1", Text: "上线了", After: 0})
	if err != nil {
		t.Fatal(err)
	}
	if again.Sent != 0 || again.Channel != "posted" || ch.n != 1 || len(users.ids) != 2 {
		t.Fatalf("retry wrote again: %+v posts %d ids %v", again, ch.n, users.ids)
	}
	rest, err := svc.Deliver(ctx, Request{EventID: "launch-1", Text: "上线了", After: first.Cursor})
	if err != nil {
		t.Fatal(err)
	}
	if rest.Status != "partial" || rest.Sent != 2 || rest.Cursor != 11 {
		t.Fatalf("%+v", rest)
	}
	end, err := svc.Deliver(ctx, Request{EventID: "launch-1", Text: "上线了", After: rest.Cursor})
	if err != nil {
		t.Fatal(err)
	}
	if end.Status != "done" || end.Sent != 0 || len(users.ids) != 4 {
		t.Fatalf("%+v ids %v", end, users.ids)
	}
}

func TestSendFailureRetriesThatUser(t *testing.T) {
	mem := NewMemory([]int64{4, 5})
	users := &recUsers{fail: map[int64]int{4: 1}}
	svc := &Service{Dir: mem, Claims: mem, Users: users, Channel: &recChannel{}}
	if _, err := svc.Deliver(context.Background(), Request{EventID: "e", Text: "hi"}); err == nil {
		t.Fatal("want unavailable")
	}
	if len(users.ids) != 0 {
		t.Fatalf("failed user was kept: %v", users.ids)
	}
	out, err := svc.Deliver(context.Background(), Request{EventID: "e", Text: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Sent != 2 || out.Status != "done" {
		t.Fatalf("%+v ids %v", out, users.ids)
	}
}

func TestChannelFailureDoesNotBlockCopies(t *testing.T) {
	mem := NewMemory([]int64{8})
	users := &recUsers{}
	ch := &recChannel{fail: 1}
	svc := &Service{Dir: mem, Claims: mem, Users: users, Channel: ch, PageSize: 2}
	first, err := svc.Deliver(context.Background(), Request{EventID: "e", Text: "hi"})
	if err != nil || first.Channel != "failed" || first.Sent != 1 || ch.n != 1 {
		t.Fatalf("%+v err %v posts %d", first, err, ch.n)
	}
	second, err := svc.Deliver(context.Background(), Request{EventID: "e", Text: "hi"})
	if err != nil || second.Channel != "posted" || second.Sent != 0 || ch.n != 2 || len(users.ids) != 1 {
		t.Fatalf("%+v err %v posts %d ids %v", second, err, ch.n, users.ids)
	}
}

func TestHTTP(t *testing.T) {
	key := bytes.Repeat([]byte{9}, 32)
	token := base64.StdEncoding.EncodeToString(key)
	mem := NewMemory([]int64{2})
	users := &recUsers{}
	h := Handler{
		Key:     key,
		Service: &Service{Dir: mem, Claims: mem, Users: users, Channel: &recChannel{}, PageSize: 2},
	}
	call := func(method, path, token string, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if call(http.MethodPost, "/nope", token, `{}`).Code != http.StatusNotFound {
		t.Fatal("path")
	}
	if call(http.MethodGet, pathPost, token, `{}`).Code != http.StatusMethodNotAllowed {
		t.Fatal("method")
	}
	if call(http.MethodPost, pathPost, "", `{"event_id":"a","text":"hi"}`).Code != http.StatusUnauthorized {
		t.Fatal("auth")
	}
	if len(users.ids) != 0 {
		t.Fatal("unauthorized wrote")
	}
	for _, body := range []string{
		`{"event_id":"","text":"hi"}`,
		`{"event_id":"ok","text":""}`,
		`{"event_id":"ok","text":"` + strings.Repeat("中", MaxTextRunes+1) + `"}`,
		`{"event_id":"bad id","text":"hi"}`,
		`{"event_id":"ok","text":"hi","cursor":"-1"}`,
		`{"event_id":"ok","text":"a\u0000b"}`,
	} {
		if rec := call(http.MethodPost, pathPost, token, body); rec.Code != http.StatusBadRequest {
			t.Fatalf("body %s -> %d", body, rec.Code)
		}
	}
	rec := call(http.MethodPost, pathPost, token, `{"event_id":"launch-1","text":"Tellomi 已上线"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["status"] != "done" || got["sent"] != float64(1) || got["cursor"] != "2" || got["channel"] != "posted" {
		t.Fatalf("%s", rec.Body.String())
	}
	if users.ids[0] != 2 {
		t.Fatalf("ids %v", users.ids)
	}
	mem.failPage = true
	rec = call(http.MethodPost, pathPost, token, `{"event_id":"launch-2","text":"x"}`)
	if rec.Code != http.StatusServiceUnavailable {
		b, _ := io.ReadAll(rec.Body)
		t.Fatalf("%d %s", rec.Code, b)
	}
}
