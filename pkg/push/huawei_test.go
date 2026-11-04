package push

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLoadHuaweiDefaultOff(t *testing.T) {
	t.Setenv(EnvHuaweiEnabled, "")
	t.Setenv(EnvHuaweiAppID, "100")
	t.Setenv(EnvHuaweiSecret, "secret")
	hw, err := LoadHuawei()
	if err != nil || hw != nil {
		t.Fatalf("off flag returned %#v %v", hw, err)
	}
	t.Setenv(EnvHuaweiEnabled, "true")
	hw, err = LoadHuawei()
	if err != nil || hw != nil {
		t.Fatalf("non-1 flag returned %#v %v", hw, err)
	}
	t.Setenv(EnvHuaweiEnabled, "1")
	t.Setenv(EnvHuaweiSecret, "")
	if _, err := LoadHuawei(); !errors.Is(err, ErrHuaweiNotConfigured) {
		t.Fatalf("missing secret: %v", err)
	}
	if strings.Contains(errString(err), "secret-value") {
		t.Fatal(err)
	}
	t.Setenv(EnvHuaweiAppID, "100")
	t.Setenv(EnvHuaweiSecret, "secret-value")
	t.Setenv(EnvHuaweiHost, "https://push.example")
	t.Setenv(EnvHuaweiTokenURL, "https://oauth.example/token")
	hw, err = LoadHuawei()
	if err != nil {
		t.Fatal(err)
	}
	if hw.appID != "100" || hw.host != "https://push.example" || hw.tokenURL != "https://oauth.example/token" || hw.secret != "secret-value" {
		t.Fatalf("%+v", hw)
	}
	if hw.client.Timeout != huaweiHTTPTimeout {
		t.Fatalf("timeout %s", hw.client.Timeout)
	}
}

func TestHuaweiSendPostsDataMessage(t *testing.T) {
	var tokenHits int32
	var gotAuth string
	var gotPath string
	var gotBody []byte
	var secretInSend bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			atomic.AddInt32(&tokenHits, 1)
			form, _ := io.ReadAll(r.Body)
			vals, _ := url.ParseQuery(string(form))
			if vals.Get("grant_type") != "client_credentials" || vals.Get("client_id") != "100" || vals.Get("client_secret") != "app-secret" {
				t.Errorf("token form %v", vals)
			}
			writeHuaweiToken(w)
			return
		}
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
		secretInSend = strings.Contains(string(gotBody), "app-secret")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"80000000","msg":"Success","requestId":"1"}`))
	}))
	defer srv.Close()
	hw := NewHuawei(srv.URL, srv.URL+"/token", "100", "app-secret")
	if err := hw.Send(context.Background(), "hms-1", "cipher-p"); err != nil {
		t.Fatal(err)
	}
	if err := hw.Send(context.Background(), "hms-1", "cipher-p"); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&tokenHits) != 1 {
		t.Fatalf("token fetches %d", tokenHits)
	}
	if gotAuth != "Bearer hms-access" {
		t.Fatalf("auth %q", gotAuth)
	}
	if secretInSend {
		t.Fatal("send body included the client secret")
	}
	if gotPath != "/v1/100/messages:send" {
		t.Fatalf("path %s", gotPath)
	}
	assertHuaweiMessage(t, gotBody, "hms-1", "cipher-p", "86400s", "IM")

	if err := hw.SendCall(context.Background(), "hms-1", "ring-p"); err != nil {
		t.Fatal(err)
	}
	assertHuaweiMessage(t, gotBody, "hms-1", "ring-p", "60s", "VOIP")
}

func TestHuaweiDropsIllegalToken(t *testing.T) {
	cases := []struct {
		name string
		body string
		drop bool
	}{
		{name: "all invalid", body: `{"code":"80300007","msg":"All the tokens are invalid","requestId":"1"}`, drop: true},
		{name: "numeric all invalid", body: `{"code":80300007,"msg":"All the tokens are invalid"}`, drop: true},
		{name: "illegal list", body: `{"code":"80100000","msg":"{\"success\":0,\"failure\":1,\"illegal_tokens\":[\"dead\"]}","requestId":"1"}`, drop: true},
		{name: "permission stays", body: `{"code":"80300002","msg":"No permission to send message"}`, drop: false},
	}
	var mu sync.Mutex
	current := cases[0].body
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			writeHuaweiToken(w)
			return
		}
		mu.Lock()
		body := current
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	hw := NewHuawei(srv.URL, srv.URL+"/token", "100", "app-secret")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mu.Lock()
			current = tc.body
			mu.Unlock()
			err := hw.Send(context.Background(), "dead", "cipher")
			var he *HuaweiError
			if !errors.As(err, &he) || he.Drop() != tc.drop {
				t.Fatalf("drop=%v err=%v", tc.drop, err)
			}
		})
	}
}

func TestDeliverDropsHuaweiTokenAndHidesPreview(t *testing.T) {
	const phrase = "secret-phrase-xyz"
	var mode string
	var mu sync.Mutex
	var posted []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			writeHuaweiToken(w)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		posted = append([]byte(nil), raw...)
		kind := mode
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if kind == "drop" {
			_, _ = w.Write([]byte(`{"code":"80300007","msg":"All the tokens are invalid"}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":"80000000","msg":"Success"}`))
	}))
	defer srv.Close()
	hw := NewHuawei(srv.URL, srv.URL+"/token", "100", "app-secret")

	secret := bytesRepeat(256)
	store := &memSettings{peer: NotifySettings{Found: true, ShowPreviews: 0}}
	ctx := context.Background()
	_ = store.Upsert(ctx, Device{AuthKeyID: 11, UserID: 9, TokenType: TokenHuawei, Token: "hms-token", Secret: secret, LangCode: "zh-Hans"})
	svc := NewService(store, Config{})
	svc.SetHuawei(hw)
	mu.Lock()
	mode = "preview"
	mu.Unlock()
	svc.Deliver(ctx, 9, textUpdateBody(phrase))
	mu.Lock()
	raw := append([]byte(nil), posted...)
	mu.Unlock()
	var body struct {
		Message struct {
			Data    string `json:"data"`
			Android struct {
				Category     string          `json:"category"`
				Notification json.RawMessage `json:"notification"`
			} `json:"android"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Message.Android.Notification) != 0 || body.Message.Android.Category != "IM" {
		t.Fatalf("%s", raw)
	}
	plain, err := DecryptNotification(secret, body.Message.Data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), phrase) {
		t.Fatalf("preview off included the message text: %s", plain)
	}
	if !strings.Contains(string(plain), "MESSAGE_NOTEXT") {
		t.Fatalf("%s", plain)
	}
	left, err := store.ListByUser(ctx, 9)
	if err != nil || len(left) != 1 {
		t.Fatalf("preview send removed the token: %+v %v", left, err)
	}

	mu.Lock()
	mode = "drop"
	mu.Unlock()
	store.peer.ShowPreviews = 1
	svc.Deliver(ctx, 9, textUpdate())
	left, err = store.ListByUser(ctx, 9)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("dead huawei token kept: %+v", left)
	}
}

func TestHuaweiPermissionErrorKeepsToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			writeHuaweiToken(w)
			return
		}
		_, _ = w.Write([]byte(`{"code":"80300002","msg":"No permission to send message"}`))
	}))
	defer srv.Close()
	store := &MemoryStore{}
	ctx := context.Background()
	_ = store.Upsert(ctx, Device{AuthKeyID: 11, UserID: 9, TokenType: TokenHuawei, Token: "hms-token", Secret: bytesRepeat(256)})
	svc := NewService(store, Config{})
	svc.SetHuawei(NewHuawei(srv.URL, srv.URL+"/token", "100", "app-secret"))
	svc.Deliver(ctx, 9, textUpdate())
	left, err := store.ListByUser(ctx, 9)
	if err != nil || len(left) != 1 || left[0].Token != "hms-token" {
		t.Fatalf("%+v %v", left, err)
	}
}

func writeHuaweiToken(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"access_token":"hms-access","expires_in":3600,"token_type":"Bearer"}`))
}

func assertHuaweiMessage(t *testing.T, raw []byte, token, data, ttl, category string) {
	t.Helper()
	var body struct {
		ValidateOnly bool `json:"validate_only"`
		Message      struct {
			Data    string   `json:"data"`
			Token   []string `json:"token"`
			Android struct {
				Urgency  string `json:"urgency"`
				TTL      string `json:"ttl"`
				Category string `json:"category"`
			} `json:"android"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body.ValidateOnly || len(body.Message.Token) != 1 || body.Message.Token[0] != token || body.Message.Data != data {
		t.Fatalf("%s", raw)
	}
	if body.Message.Android.Urgency != "HIGH" || body.Message.Android.TTL != ttl || body.Message.Android.Category != category {
		t.Fatalf("%s", raw)
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
