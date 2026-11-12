package tellomiai

import (
	"context"
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

func TestTranslateDoesNotCallBailian(t *testing.T) {
	t.Setenv(EnvTranslate, "1")
	t.Setenv(EnvAPIKey, "test-key")
	t.Setenv(EnvTranslateURL, "")
	SetHook(nil)
	cachedHealth = healthCache{}
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls++
	}))
	t.Cleanup(srv.Close)
	t.Setenv(EnvBaseURL, srv.URL)
	if _, err := Rewrite(context.Background(), Job{Mode: ModeTranslate, Texts: []string{"hello"}, ToLang: "zh-hans"}); err != ErrDisabled {
		t.Fatalf("%v", err)
	}
	if calls != 0 {
		t.Fatalf("bailian calls %d", calls)
	}
}

func TestTranslateSidecarFailureDoesNotFallBack(t *testing.T) {
	t.Setenv(EnvTranslate, "1")
	t.Setenv(EnvAPIKey, "test-key")
	SetHook(nil)
	var bailian int
	bail := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		bailian++
	}))
	t.Cleanup(bail.Close)
	t.Setenv(EnvBaseURL, bail.URL)
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(local.Close)
	t.Setenv(EnvTranslateURL, local.URL)
	cachedHealth = healthCache{}
	if _, err := Rewrite(context.Background(), Job{Mode: ModeTranslate, Texts: []string{"hello"}, ToLang: "zh"}); err != ErrSidecarBusy {
		t.Fatalf("%v", err)
	}
	if bailian != 0 {
		t.Fatalf("fallback calls %d", bailian)
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
