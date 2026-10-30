package resend

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFormatFrom(t *testing.T) {
	if got := formatFrom("Tellomi", "noreply@tellomi.app"); got != "Tellomi <noreply@tellomi.app>" {
		t.Fatal(got)
	}
	if got := formatFrom("A\r\n<B>", "noreply@tellomi.app"); got != "AB <noreply@tellomi.app>" {
		t.Fatal(got)
	}
	if got := formatFrom("", "noreply@tellomi.app"); got != "noreply@tellomi.app" {
		t.Fatal(got)
	}
}

func TestMessageTextIncludesCodeInBothLanguages(t *testing.T) {
	got := messageText("654321")
	if !strings.Contains(got, "您的验证码为：654321，请勿泄露于他人！") {
		t.Fatal(got)
	}
	if !strings.Contains(got, "Your verification code is 654321. Do not share it.") {
		t.Fatal(got)
	}
}

func TestSendCodeRequiresAPIKeyBeforeDial(t *testing.T) {
	c := &Client{
		From:     "noreply@tellomi.app",
		FromName: "Tellomi",
		HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("missing api key must not dial")
			return nil, nil
		})},
	}
	if err := c.SendCode(context.Background(), "a@b.cn", "123456"); err == nil || !strings.Contains(err.Error(), "api key") {
		t.Fatal(err)
	}
}

func TestSendCodePostsEmail(t *testing.T) {
	var (
		gotAuth string
		gotPath string
		gotBody sendRequest
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Error(err)
		}
		if strings.Contains(string(raw), "re_test_key") {
			t.Error("api key leaked into the body")
		}
		_, _ = w.Write([]byte(`{"id":"email_123"}`))
	}))
	defer srv.Close()

	c := &Client{
		APIKey:   "re_test_key",
		From:     "noreply@tellomi.app",
		FromName: "Tellomi",
		Endpoint: srv.URL + "/emails",
		HTTP:     srv.Client(),
	}
	if err := c.SendCode(context.Background(), "user@example.com", "654321"); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer re_test_key" {
		t.Fatalf("auth %q", gotAuth)
	}
	if gotPath != "/emails" {
		t.Fatal(gotPath)
	}
	if len(gotBody.To) != 1 || gotBody.To[0] != "user@example.com" {
		t.Fatalf("to %#v", gotBody.To)
	}
	if gotBody.From != "Tellomi <noreply@tellomi.app>" {
		t.Fatalf("from %q", gotBody.From)
	}
	if gotBody.Subject != otpSubject {
		t.Fatal(gotBody.Subject)
	}
	if !strings.Contains(gotBody.Text, "654321") || !strings.Contains(gotBody.Text, "您的验证码为：") || !strings.Contains(gotBody.Text, "Your verification code is 654321") {
		t.Fatal(gotBody.Text)
	}
}

func TestSendCodeUsesDefaultEndpoint(t *testing.T) {
	var gotURL string
	c := &Client{
		APIKey:   "re_test_key",
		From:     "noreply@tellomi.app",
		FromName: "Tellomi",
		HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotURL = r.URL.String()
			if r.Header.Get("Authorization") != "Bearer re_test_key" {
				t.Fatalf("auth %q", r.Header.Get("Authorization"))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"id":"email_123"}`)),
				Header:     make(http.Header),
				Request:    r,
			}, nil
		})},
	}
	if err := c.SendCode(context.Background(), "user@example.com", "654321"); err != nil {
		t.Fatal(err)
	}
	if gotURL != defaultEndpoint {
		t.Fatal(gotURL)
	}
}

func TestSendCodeResendError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"statusCode":422,"name":"validation_error","message":"Invalid from field"}`))
	}))
	defer srv.Close()

	c := &Client{
		APIKey:   "re_test_key",
		From:     "noreply@tellomi.app",
		Endpoint: srv.URL,
		HTTP:     srv.Client(),
	}
	err := c.SendCode(context.Background(), "user@example.com", "654321")
	if err == nil || !strings.Contains(err.Error(), "validation_error") || !strings.Contains(err.Error(), "Invalid from field") {
		t.Fatal(err)
	}
	if strings.Contains(err.Error(), "654321") || strings.Contains(err.Error(), "re_test_key") {
		t.Fatal(err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
