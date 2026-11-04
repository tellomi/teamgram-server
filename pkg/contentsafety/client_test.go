package contentsafety

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestInterpret(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"empty labels", 200, `{"Code":200,"Data":{"labels":"","reason":""},"Message":"OK","RequestId":"r1"}`, Allowed},
		{"classic label", 200, `{"Code":200,"Data":{"Labels":"political_content"},"Message":"OK","RequestId":"r2"}`, Blocked},
		{"plus pass", 200, `{"Code":200,"Data":{"RiskLevel":"none","Result":[{"Label":"nonLabel"}]},"RequestId":"r3"}`, Allowed},
		{"plus high", 200, `{"Code":200,"Data":{"RiskLevel":"high","Result":[{"Label":"political_entity"}]},"RequestId":"r4"}`, Blocked},
		{"plus medium", 200, `{"Code":200,"Data":{"RiskLevel":"medium","Result":[{"Label":"nonLabel"}]},"RequestId":"r5"}`, Blocked},
		{"string code", 200, `{"Code":"200","Data":{"labels":"ad"},"RequestId":"r6"}`, Blocked},
		{"api error", 200, `{"Code":400,"Message":"service is invalid","RequestId":"r7"}`, Fallback},
		{"http 503", 503, `{"Code":200,"Data":{"labels":"ad"}}`, Fallback},
		{"not json", 200, `nope`, Fallback},
		{"encoded data", 200, `{"Code":200,"Data":"{\"labels\":\"violence\"}","RequestId":"r8"}`, Blocked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, labels, _, _, _ := interpret(tc.status, []byte(tc.body))
			if got != tc.want {
				t.Fatalf("got %s labels %s", got, labels)
			}
			if got == Blocked && strings.Contains(labels, "小") {
				t.Fatal("label echoed the input")
			}
		})
	}
}

func TestDecideOffDoesNotCall(t *testing.T) {
	t.Setenv(envEnabled, "")
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hit = true
	}))
	defer srv.Close()
	t.Setenv(envEndpoint, srv.URL)
	if got := Decide(context.Background(), "小明"); got != Allowed || hit {
		t.Fatalf("got %s hit %v", got, hit)
	}
}

func TestDecideASCIIDoesNotCall(t *testing.T) {
	t.Setenv(envEnabled, "aliyun")
	t.Setenv(envKeyID, "id")
	t.Setenv(envSecret, "secret")
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hit = true
	}))
	defer srv.Close()
	t.Setenv(envEndpoint, srv.URL)
	if got := Decide(context.Background(), "admin"); got != Allowed || hit {
		t.Fatalf("got %s hit %v", got, hit)
	}
}

func TestDecideMissingKeyFallsOpen(t *testing.T) {
	t.Setenv(envEnabled, "1")
	t.Setenv(envKeyID, "")
	t.Setenv(envSecret, "")
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "")
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "")
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hit = true
	}))
	defer srv.Close()
	t.Setenv(envEndpoint, srv.URL)
	if got := Decide(context.Background(), "小明"); got != Fallback || hit {
		t.Fatalf("got %s hit %v", got, hit)
	}
}

func TestDecideAliyunBlockAndAllow(t *testing.T) {
	var action, service, content string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.Form.Get("Signature") == "" || r.Form.Get("AccessKeyId") != "id" {
			t.Errorf("signature or key id missing")
		}
		if strings.Contains(r.Form.Encode(), "secret") {
			t.Error("request contains the secret")
		}
		action = r.Form.Get("Action")
		service = r.Form.Get("Service")
		content = r.Form.Get("ServiceParameters")
		if strings.Contains(content, "拦") {
			_, _ = w.Write([]byte(`{"Code":200,"Data":{"labels":"political_content"},"Message":"OK","RequestId":"block"}`))
			return
		}
		_, _ = w.Write([]byte(`{"Code":200,"Data":{"labels":""},"Message":"OK","RequestId":"pass"}`))
	}))
	defer srv.Close()
	t.Setenv(envEnabled, "aliyun")
	t.Setenv(envEndpoint, srv.URL)
	t.Setenv(envKeyID, "id")
	t.Setenv(envSecret, "secret")

	if got := Decide(context.Background(), "拦下这个名字"); got != Blocked {
		t.Fatalf("block: %s", got)
	}
	if action != "TextModeration" || service != "nickname_detection" || !strings.Contains(content, "拦下这个名字") {
		t.Fatalf("request action=%s service=%s params=%s", action, service, content)
	}
	if got := Decide(context.Background(), "小明"); got != Allowed {
		t.Fatalf("allow: %s", got)
	}
}

func TestDecideBailianUsesPlus(t *testing.T) {
	var action, service string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action = r.Form.Get("Action")
		service = r.Form.Get("Service")
		_, _ = w.Write([]byte(`{"Code":200,"Data":{"RiskLevel":"high","Result":[{"Label":"political_entity","RiskWords":"词"}]},"RequestId":"b"}`))
	}))
	defer srv.Close()
	t.Setenv(envEnabled, "bailian")
	t.Setenv(envEndpoint, srv.URL)
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_ID", "id")
	t.Setenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET", "secret")
	t.Setenv(envKeyID, "")
	t.Setenv(envSecret, "")

	if got := Decide(context.Background(), "测试文本"); got != Blocked {
		t.Fatalf("got %s", got)
	}
	if action != "TextModerationPlus" || service != "nickname_detection_pro" {
		t.Fatalf("action=%s service=%s", action, service)
	}
}

func TestDecideTimeoutFallsOpen(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-time.After(5 * time.Second):
			_, _ = w.Write([]byte(`{"Code":200,"Data":{"labels":"ad"}}`))
		}
	}))
	defer srv.Close()
	t.Setenv(envEnabled, "1")
	t.Setenv(envEndpoint, srv.URL)
	t.Setenv(envKeyID, "id")
	t.Setenv(envSecret, "secret")
	t.Setenv(envTimeout, "200ms")
	start := time.Now()
	got := Decide(context.Background(), "超时名字")
	close(release)
	if got != Fallback {
		t.Fatalf("got %s", got)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("timeout took %s", time.Since(start))
	}
}

func TestDecideHTTPErrorFallsOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"Code":200,"Data":{"labels":"ad"}}`))
	}))
	defer srv.Close()
	t.Setenv(envEnabled, "1")
	t.Setenv(envEndpoint, srv.URL)
	t.Setenv(envKeyID, "id")
	t.Setenv(envSecret, "secret")
	if got := Decide(context.Background(), "网关名字"); got != Fallback {
		t.Fatalf("got %s", got)
	}
}
