package postal

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendURL(t *testing.T) {
	if got := sendURL("https://postal.internal/"); got != "https://postal.internal/api/v1/send/message" {
		t.Fatal(got)
	}
	full := "https://postal.internal/api/v1/send/message"
	if got := sendURL(full); got != full {
		t.Fatal(got)
	}
}

func TestFormatFrom(t *testing.T) {
	if got := formatFrom("Tellomi", "noreply@tellomi.app"); got != "Tellomi <noreply@tellomi.app>" {
		t.Fatal(got)
	}
	if got := formatFrom("A\r\n<B>", "noreply@tellomi.app"); got != "AB <noreply@tellomi.app>" {
		t.Fatal(got)
	}
}

func TestSendCodeRequiresConfigBeforeDial(t *testing.T) {
	c := &Client{From: "noreply@tellomi.app", FromName: "Tellomi"}
	if err := c.SendCode(context.Background(), "a@b.cn", "123456"); err == nil || !strings.Contains(err.Error(), "base url") {
		t.Fatal(err)
	}
	c.BaseURL = "https://postal.internal"
	if err := c.SendCode(context.Background(), "a@b.cn", "123456"); err == nil || !strings.Contains(err.Error(), "api key") {
		t.Fatal(err)
	}
}

func TestSendCodePostsMessage(t *testing.T) {
	var (
		gotKey  string
		gotPath string
		gotBody sendRequest
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Server-API-Key")
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Error(err)
		}
		if strings.Contains(string(raw), "server-key") {
			t.Error("api key leaked into the body")
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"message_id":"m1"}}`))
	}))
	defer srv.Close()

	c := &Client{
		BaseURL:  srv.URL,
		APIKey:   "server-key",
		From:     "noreply@tellomi.app",
		FromName: "Tellomi",
		HTTP:     srv.Client(),
	}
	if err := c.SendCode(context.Background(), "user@example.com", "654321"); err != nil {
		t.Fatal(err)
	}
	if gotKey != "server-key" {
		t.Fatalf("key %q", gotKey)
	}
	if gotPath != "/api/v1/send/message" {
		t.Fatal(gotPath)
	}
	if len(gotBody.To) != 1 || gotBody.To[0] != "user@example.com" {
		t.Fatalf("to %#v", gotBody.To)
	}
	if gotBody.From != "Tellomi <noreply@tellomi.app>" || gotBody.Sender != "noreply@tellomi.app" {
		t.Fatalf("from %q sender %q", gotBody.From, gotBody.Sender)
	}
	if !strings.Contains(gotBody.PlainBody, "654321") || !strings.Contains(gotBody.PlainBody, "您的验证码为：") {
		t.Fatal(gotBody.PlainBody)
	}
}

func TestSendCodePostalError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"status":"error","data":{"code":"ValidationError","message":"From is not allowed"}}`))
	}))
	defer srv.Close()

	c := &Client{
		BaseURL: srv.URL,
		APIKey:  "k",
		From:    "noreply@tellomi.app",
		HTTP:    srv.Client(),
	}
	err := c.SendCode(context.Background(), "user@example.com", "654321")
	if err == nil || !strings.Contains(err.Error(), "ValidationError") || !strings.Contains(err.Error(), "From is not allowed") {
		t.Fatal(err)
	}
}
