package push

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLoadXiaomiDefaultOff(t *testing.T) {
	t.Setenv(EnvXiaomiEnabled, "")
	t.Setenv(EnvXiaomiSecret, "secret")
	t.Setenv(EnvXiaomiPackage, "app.tellomi.chat")
	xm, err := LoadXiaomi()
	if err != nil || xm != nil {
		t.Fatalf("off flag returned %#v %v", xm, err)
	}
	t.Setenv(EnvXiaomiEnabled, "1")
	t.Setenv(EnvXiaomiSecret, "")
	if _, err := LoadXiaomi(); err != ErrXiaomiNotConfigured {
		t.Fatalf("missing secret: %v", err)
	}
}

func TestXiaomiSendPostsRegID(t *testing.T) {
	var got url.Values
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != xiaomiRegIDPath {
			t.Errorf("path %s", r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"ok","code":0,"description":"成功"}`))
	}))
	defer srv.Close()
	xm := NewXiaomi(srv.URL, "app-secret", "app.tellomi.chat")
	err := xm.Send(context.Background(), "reg-1", XiaomiMessage{
		Title: "林", Description: "你好", Payload: "cipher", Silent: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if auth != "key=app-secret" {
		t.Fatalf("auth %q", auth)
	}
	if got.Get("registration_id") != "reg-1" || got.Get("restricted_package_name") != "app.tellomi.chat" {
		t.Fatalf("%v", got)
	}
	if got.Get("payload") != "cipher" || got.Get("title") != "林" || got.Get("description") != "你好" {
		t.Fatalf("%v", got)
	}
	if got.Get("pass_through") != "0" || got.Get("notify_type") != "-1" {
		t.Fatalf("pass %s notify %s", got.Get("pass_through"), got.Get("notify_type"))
	}
}

func TestXiaomiInvalidRegIDDrops(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":"error","code":20301,"reason":"invalidRegId","description":"Invalid registration id"}`))
	}))
	defer srv.Close()
	xm := NewXiaomi(srv.URL, "app-secret", "app.tellomi.chat")
	err := xm.Send(context.Background(), "dead", XiaomiMessage{Title: "T", Description: "D", Payload: "p"})
	var xe *XiaomiError
	if !errors.As(err, &xe) || !xe.Drop() || xe.Code != XiaomiInvalidRegID {
		t.Fatalf("%v", err)
	}
}

func TestDeliverSendsXiaomiAndDropsDeadRegID(t *testing.T) {
	store := &memSettings{peer: NotifySettings{Found: true, ShowPreviews: 1}}
	ctx := context.Background()
	secret := bytesRepeat(256)
	_ = store.Upsert(ctx, Device{
		AuthKeyID: 12, UserID: 9, TokenType: TokenXiaomi, Token: "mi-token", Secret: secret, LangCode: "zh-Hans",
	})
	cap := &captureXiaomi{}
	svc := NewService(store, Config{})
	svc.SetXiaomi(cap)
	svc.Deliver(ctx, 9, textUpdate())
	cap.mu.Lock()
	token, payload, title := cap.token, cap.msg.Payload, cap.msg.Title
	cap.mu.Unlock()
	if token != "mi-token" || payload == "" || title == "" {
		t.Fatalf("token %q payload %d title %q", token, len(payload), title)
	}
	plain, err := DecryptNotification(secret, payload)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plain), "loc_key") {
		t.Fatalf("plain %s", plain)
	}

	cap.err = &XiaomiError{Code: XiaomiInvalidRegID, Reason: "invalidRegId"}
	svc.Deliver(ctx, 9, textUpdate())
	left, err := store.ListByUser(ctx, 9)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("dead regid kept: %+v", left)
	}
}

func TestXiaomiUnconfiguredDoesNotSend(t *testing.T) {
	store := &memSettings{peer: NotifySettings{Found: true, ShowPreviews: 1}}
	_ = store.Upsert(context.Background(), Device{
		AuthKeyID: 1, UserID: 9, TokenType: TokenXiaomi, Token: "mi",
	})
	svc := NewService(store, Config{})
	svc.Deliver(context.Background(), 9, textUpdate())
}

func TestMutedChatSkipsXiaomi(t *testing.T) {
	store := &memSettings{peer: NotifySettings{Found: true, ShowPreviews: 1, MuteUntil: int32(time.Now().Unix()) + 3600}}
	_ = store.Upsert(context.Background(), Device{
		AuthKeyID: 1, UserID: 9, TokenType: TokenXiaomi, Token: "mi", Secret: bytesRepeat(256),
	})
	cap := &captureXiaomi{}
	svc := NewService(store, Config{})
	svc.SetXiaomi(cap)
	svc.Deliver(context.Background(), 9, textUpdate())
	if cap.token != "" {
		t.Fatal("muted chat was pushed to xiaomi")
	}
}

type captureXiaomi struct {
	mu    sync.Mutex
	token string
	msg   XiaomiMessage
	err   error
}

func (c *captureXiaomi) Send(_ context.Context, token string, msg XiaomiMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = token
	c.msg = msg
	return c.err
}
