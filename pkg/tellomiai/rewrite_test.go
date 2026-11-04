package tellomiai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/contentsafety"
)

func TestRewriteFlagOffDoesNotCallHook(t *testing.T) {
	t.Setenv(EnvTranslate, "")
	t.Setenv(EnvCompose, "")
	t.Setenv(EnvAPIKey, "")
	SetHook(func(context.Context, Job) ([]string, error) {
		t.Fatal("hook called while the flag is off")
		return nil, nil
	})
	t.Cleanup(func() { SetHook(nil) })
	if _, err := Rewrite(context.Background(), Job{Mode: ModeTranslate, Texts: []string{"hello"}, ToLang: "zh"}); err != ErrDisabled {
		t.Fatalf("translate: %v", err)
	}
	if _, err := Rewrite(context.Background(), Job{Mode: ModeCompose, Texts: []string{"hello"}, Proofread: true}); err != ErrDisabled {
		t.Fatalf("compose: %v", err)
	}
}

func TestRewriteHookAndSafety(t *testing.T) {
	t.Setenv(EnvTranslate, "1")
	t.Setenv(EnvAPIKey, "test-key")
	SetHook(func(_ context.Context, job Job) ([]string, error) {
		if len(job.Texts) != 1 || job.Texts[0] != "hello" || job.Mode != ModeTranslate {
			t.Fatalf("job %#v", job)
		}
		return []string{"你好"}, nil
	})
	t.Cleanup(func() {
		SetHook(nil)
		SetModerate(nil)
	})
	got, err := Rewrite(context.Background(), Job{Mode: ModeTranslate, Texts: []string{"hello"}, ToLang: "zh-hans"})
	if err != nil || len(got) != 1 || got[0] != "你好" {
		t.Fatalf("%v %#v", err, got)
	}
	SetModerate(func(context.Context, string) string { return contentsafety.Blocked })
	if _, err := Rewrite(context.Background(), Job{Mode: ModeTranslate, Texts: []string{"hello"}, ToLang: "zh"}); err != ErrSafety {
		t.Fatalf("safety: %v", err)
	}
	long := strings.Repeat("a", MaxRewriteRunes+1)
	if _, err := Rewrite(context.Background(), Job{Mode: ModeTranslate, Texts: []string{long}, ToLang: "en"}); err != ErrTooLong {
		t.Fatalf("long: %v", err)
	}
}

func TestRewritePrimaryFallsBackToDeepSeek(t *testing.T) {
	t.Setenv(EnvTranslate, "1")
	t.Setenv(EnvAPIKey, "test-key")
	t.Setenv(EnvModelPrimary, defaultPrimary)
	t.Setenv(EnvModelFallback, defaultFallback)
	SetHook(nil)
	var models []string
	var system string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("body: %v", err)
		}
		models = append(models, body.Model)
		for _, msg := range body.Messages {
			if msg.Role == "system" {
				system = msg.Content
			}
		}
		if strings.Contains(string(raw), "test-key") {
			http.Error(w, "key echoed", http.StatusInternalServerError)
			return
		}
		if body.Model == defaultPrimary {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"unavailable"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"[\"你好\"]"}}],"usage":{"total_tokens":3}}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv(EnvBaseURL, srv.URL)
	got, err := Rewrite(context.Background(), Job{Mode: ModeTranslate, Texts: []string{"hello"}, ToLang: "zh-hans"})
	if err != nil || len(got) != 1 || got[0] != "你好" {
		t.Fatalf("%v %#v", err, got)
	}
	if len(models) != 2 || models[0] != defaultPrimary || models[1] != defaultFallback {
		t.Fatalf("models %v", models)
	}
	if system == "" || strings.Contains(system, "官方助手") || !strings.Contains(system, "JSON") {
		t.Fatalf("system %q", system)
	}
}

func TestRewriteSafetyDoesNotFallBack(t *testing.T) {
	t.Setenv(EnvTranslate, "1")
	t.Setenv(EnvAPIKey, "test-key")
	t.Setenv(EnvModelPrimary, defaultPrimary)
	t.Setenv(EnvModelFallback, defaultFallback)
	SetHook(nil)
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"data_inspection_failed","message":"Input data may contain inappropriate content."}}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv(EnvBaseURL, srv.URL)
	if _, err := Rewrite(context.Background(), Job{Mode: ModeTranslate, Texts: []string{"hello"}, ToLang: "en"}); err != ErrSafety {
		t.Fatalf("%v", err)
	}
	if calls != 1 {
		t.Fatalf("calls %d", calls)
	}
}

func TestParseStringArray(t *testing.T) {
	got, err := parseStringArray("```json\n[\"你好\", \"世界\"]\n```", 2)
	if err != nil || len(got) != 2 || got[0] != "你好" || got[1] != "世界" {
		t.Fatalf("%v %#v", err, got)
	}
	if _, err := parseStringArray(`["only"]`, 2); err == nil {
		t.Fatal("length mismatch accepted")
	}
}
