package helpdesk

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

const publishedManifestJSON = `{
  "platform": "android",
  "channel": "release",
  "versionName": "14.0.6",
  "versionCode": 1802599,
  "url": "https://www.tellomi.cn/dl/android/14.0.6-1802599/Tellomi.apk",
  "fallbackUrl": "https://dl.tellomi.cn/android/14.0.6-1802599/Tellomi.apk",
  "sha256sum": "30a1a55a465e281e39808fcf251a8743558a6dc2639de17118b240c7927a6e0d",
  "size": 71094607,
  "files": [
    {
      "sha256": "30a1a55a465e281e39808fcf251a8743558a6dc2639de17118b240c7927a6e0d",
      "url": "https://example.invalid/ignored.apk"
    }
  ]
}`

func TestParsePublishedManifest(t *testing.T) {
	rel, err := parseManifest([]byte(publishedManifestJSON))
	if err != nil {
		t.Fatal(err)
	}
	if rel.Version != "14.0.6" || rel.CannotSkip || rel.Text != "" {
		t.Fatalf("%#v", rel)
	}
	if rel.URL != "https://www.tellomi.cn/dl/android/14.0.6-1802599/Tellomi.apk" {
		t.Fatal(rel.URL)
	}
	got := AppUpdate("14.0.5", "", rel)
	if got.GetPredicateName() != mtproto.Predicate_help_appUpdate || got.GetVersion() != "14.0.6" || got.GetCanNotSkip() {
		t.Fatalf("%s %s skip=%v", got.GetPredicateName(), got.GetVersion(), got.GetCanNotSkip())
	}
	if got.GetText() != "A new version of Tellomi is available." {
		t.Fatal(got.GetText())
	}
	if got.GetUrl().GetValue() != rel.URL {
		t.Fatal(got.GetUrl())
	}
	if AppUpdate("14.0.6", "", rel).GetPredicateName() != mtproto.Predicate_help_noAppUpdate {
		t.Fatal("same version")
	}
	ios := AppUpdateForSession(UpdateClient{Known: true, LangPack: "ios", AppVersion: "14.0.1"}, "", rel)
	if ios.GetPredicateName() != mtproto.Predicate_help_noAppUpdate {
		t.Fatal(ios.GetPredicateName())
	}

	withLog, err := parseManifest([]byte(`{"versionName":"14.0.7","versionCode":"1802600","url":" https://example.com/a.apk ","changelog":"  Fixes  ","size":"1"}`))
	if err != nil || withLog.Text != "Fixes" || withLog.URL != "https://example.com/a.apk" || withLog.Version != "14.0.7" {
		t.Fatalf("%v %#v", err, withLog)
	}
	fallback, err := parseManifest([]byte(`{"versionName":"14.0.7","fallbackUrl":"https://dl.example/a.apk","changelog":{"en":"no"}}`))
	if err != nil || fallback.URL != "https://dl.example/a.apk" || fallback.Text != "" {
		t.Fatalf("%v %#v", err, fallback)
	}
	bare, err := parseManifest([]byte(`{"versionName":"1802599","url":"https://example.com/a.apk"}`))
	if err != nil {
		t.Fatal(err)
	}
	if AppUpdate("14.0.6", "", bare).GetPredicateName() != mtproto.Predicate_help_noAppUpdate {
		t.Fatal("bare build")
	}
	if _, err := parseManifest([]byte(`{"url":"https://example.com/a.apk"}`)); err == nil {
		t.Fatal("missing versionName")
	}
	if _, err := parseManifest([]byte(`{`)); err == nil {
		t.Fatal("invalid json")
	}

	forced, err := parseManifest([]byte(`{"versionName":"14.0.7","url":"https://example.com/a.apk","mandatory":true}`))
	if err != nil || !forced.CannotSkip {
		t.Fatalf("%v %#v", err, forced)
	}
	soft, err := parseManifest([]byte(`{"versionName":"14.0.7","url":"https://example.com/a.apk","mandatory":false}`))
	if err != nil || soft.CannotSkip {
		t.Fatalf("%v %#v", err, soft)
	}
	quoted, err := parseManifest([]byte(`{"versionName":"14.0.7","url":"https://example.com/a.apk","mandatory":"true"}`))
	if err != nil || quoted.CannotSkip {
		t.Fatalf("%v %#v", err, quoted)
	}
}

func TestManifestSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "latest.json")
	if err := os.WriteFile(path, []byte(`{"versionName":"14.0.6","url":"https://example.com/a.apk"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := []string{filepath.Join(dir, "missing.json")}
	local := []string{path, filepath.Join(dir, "missing.json")}

	got, ok := pickManifestSource("", true, local)
	if !ok || got != path {
		t.Fatalf("local %q ok=%v", got, ok)
	}
	got, ok = pickManifestSource("", false, missing)
	if !ok || got != defaultManifestURL {
		t.Fatalf("url %q ok=%v", got, ok)
	}
	got, ok = pickManifestSource(" https://updates.example/latest.json ", true, local)
	if !ok || got != "https://updates.example/latest.json" {
		t.Fatalf("explicit %q ok=%v", got, ok)
	}
	for _, off := range []string{"off", "OFF", "none", "-"} {
		if _, ok := pickManifestSource(off, true, local); ok {
			t.Fatalf("%s enabled the feed", off)
		}
	}
}

func TestManifestFileAndHTTP(t *testing.T) {
	body := []byte(`{"versionName":"14.0.8","url":"https://example.com/a.apk","changelog":"Fixes"}`)
	path := filepath.Join(t.TempDir(), "latest.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	rel, err := loadCachedManifest(path)
	if err != nil || rel.Version != "14.0.8" || rel.Text != "Fixes" || rel.CannotSkip {
		t.Fatalf("%v %#v", err, rel)
	}
	if _, err := loadCachedManifest(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing file")
	}
	if _, err := readManifestBody(context.Background(), "file:///tmp/latest.json"); err == nil {
		t.Fatal("file scheme")
	}

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("User-Agent") != "tellomi-bff help.getAppUpdate" {
			t.Errorf("user agent %q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	got, err := loadCachedManifest(srv.URL)
	if err != nil || got.Version != "14.0.8" {
		t.Fatalf("%v %#v", err, got)
	}
	if _, err := loadCachedManifest(srv.URL); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits %d", hits.Load())
	}
}

func TestManifestCacheExpiryAndFailure(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n < 3 {
			http.Error(w, "nope", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"versionName":"14.0.9","url":"https://example.com/a.apk"}`))
	}))
	t.Cleanup(srv.Close)

	start := time.Unix(1_700_000_000, 0)
	var cur atomic.Int64
	cur.Store(start.UnixNano())
	oldNow := manifestNow
	manifestNow = func() time.Time { return time.Unix(0, cur.Load()) }
	t.Cleanup(func() { manifestNow = oldNow })

	if _, err := loadCachedManifest(srv.URL); err == nil {
		t.Fatal("expected HTTP error")
	}
	if _, err := loadCachedManifest(srv.URL); err == nil {
		t.Fatal("expected cached HTTP error")
	}
	if hits.Load() != 1 {
		t.Fatalf("hits %d", hits.Load())
	}
	cur.Store(start.Add(manifestFailureTTL + time.Second).UnixNano())
	if _, err := loadCachedManifest(srv.URL); err == nil {
		t.Fatal("expected second HTTP error")
	}
	if hits.Load() != 2 {
		t.Fatalf("hits %d", hits.Load())
	}
	cur.Store(start.Add(2*manifestFailureTTL + 2*time.Second).UnixNano())
	rel, err := loadCachedManifest(srv.URL)
	if err != nil || rel.Version != "14.0.9" {
		t.Fatalf("%v %#v", err, rel)
	}
	if hits.Load() != 3 {
		t.Fatalf("hits %d", hits.Load())
	}
	cur.Store(start.Add(2*manifestFailureTTL + 2*time.Second + manifestSuccessTTL - time.Second).UnixNano())
	if _, err := loadCachedManifest(srv.URL); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 3 {
		t.Fatalf("success cache hits %d", hits.Load())
	}
}

func TestManifestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	old := manifestHTTPTimeout
	manifestHTTPTimeout = 150 * time.Millisecond
	t.Cleanup(func() { manifestHTTPTimeout = old })
	if _, err := loadCachedManifest(srv.URL + "/slow"); err == nil {
		t.Fatal("expected timeout")
	}
}

func TestManifestSingleflight(t *testing.T) {
	var hits atomic.Int32
	block := make(chan struct{})
	var closeBlock sync.Once
	unblock := func() { closeBlock.Do(func() { close(block) }) }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			<-block
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"versionName":"14.1.0","url":"https://example.com/a.apk"}`))
	}))
	t.Cleanup(func() {
		unblock()
		srv.Close()
	})

	const n = 8
	errCh := make(chan error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			rel, err := loadCachedManifest(srv.URL + "/once")
			if err != nil || rel == nil || rel.Version != "14.1.0" {
				errCh <- errOrFmt(err, rel)
				return
			}
			errCh <- nil
		}()
	}
	deadline := time.Now().Add(2 * time.Second)
	for hits.Load() == 0 {
		if time.Now().After(deadline) {
			unblock()
			t.Fatal("fetch did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	if hits.Load() != 1 {
		unblock()
		t.Fatalf("hits before release %d", hits.Load())
	}
	unblock()
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("hits %d", hits.Load())
	}
}

func errOrFmt(err error, rel *Release) error {
	if err != nil {
		return err
	}
	if rel == nil {
		return fmt.Errorf("nil release")
	}
	return fmt.Errorf("version %s", rel.Version)
}
