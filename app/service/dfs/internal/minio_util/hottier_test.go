package minio_util

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func hotTestConfig(t *testing.T, origin *httptest.Server, tr *http.Transport, dir, proxy string) MinioConfig {
	t.Helper()
	cfg := objectConfig(origin, tr)
	cfg.HTTPProxy = proxy
	cfg.hot = defaultHotSettings()
	cfg.hot.Enabled = true
	cfg.hot.Dir = dir
	cfg.hot.noLoop = true
	cfg.hot.Quiet = 0
	cfg.hot.MaxAge = time.Hour
	return cfg
}

func TestHotTierDisabledMatchesDirectPut(t *testing.T) {
	fake := newS3Fake()
	fake.allow("tellomi-attachments", "tellomi-public-media", "tellomi-backups")
	origin, cfg := fake.server(t)
	defer origin.Close()
	dir := t.TempDir()
	cfg.hot.Dir = dir
	cfg.hot.Enabled = false
	client, err := NewMinioClient(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = client.PutPhotoFile(ctx, "m/off.dat", []byte("from-r2")); err != nil {
		t.Fatal(err)
	}
	if client.hot != nil {
		t.Fatal("hot tier was installed while the flag was off")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("local files written with the flag off: %v", entries)
	}
	if !fake.has("tellomi-attachments", "photos/m/off.dat", "from-r2") {
		t.Fatal("object was not stored on the origin")
	}
	got, err := client.GetPhotoFile(ctx, "m/off.dat", 0, 32)
	if err != nil || string(got) != "from-r2" {
		t.Fatalf("get %q %v", got, err)
	}
}

func TestHotTierRequiresProxyAndDoesNotDial(t *testing.T) {
	fake := newS3Fake()
	fake.allow("tellomi-attachments", "tellomi-public-media", "tellomi-backups")
	origin := httptest.NewTLSServer(fake)
	defer origin.Close()
	var direct atomic.Int32
	tr := tlsTransport(t, origin.Listener.Addr().String(), &direct, 0)
	cfg := objectConfig(origin, tr)
	cfg.hot = defaultHotSettings()
	cfg.hot.Enabled = true
	cfg.hot.Dir = t.TempDir()
	cfg.hot.noLoop = true
	cfg.HTTPProxy = ""
	_, err := NewMinioClient(&cfg)
	if err == nil {
		t.Fatal("hot tier started without a proxy")
	}
	if !strings.Contains(err.Error(), EnvHTTPProxy) {
		t.Fatalf("err %v", err)
	}
	if direct.Load() != 0 {
		t.Fatalf("direct dials %d", direct.Load())
	}
}

func TestHotTierLocalHitFallbackMigrateAndPurge(t *testing.T) {
	fake := newS3Fake()
	fake.allow("tellomi-attachments", "tellomi-public-media", "tellomi-backups")
	rec := &headerRecorder{inner: fake}
	origin := httptest.NewTLSServer(rec)
	defer origin.Close()
	proxy, connects := connectProxy(t, "tunnel")
	defer proxy.Close()
	var direct atomic.Int32
	tr := tlsTransport(t, origin.Listener.Addr().String(), &direct, 0)
	dir := t.TempDir()
	cfg := hotTestConfig(t, origin, tr, dir, proxy.URL)
	client, err := NewMinioClient(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	body := []byte("hot-photo-bytes")
	if _, err = client.PutPhotoFile(ctx, "m/1.dat", body); err != nil {
		t.Fatal(err)
	}
	if fake.has("tellomi-attachments", "photos/m/1.dat", string(body)) {
		t.Fatal("upload wrote through to R2 while the hot tier is on")
	}
	got, err := client.GetPhotoFile(ctx, "m/1.dat", 0, 32)
	if err != nil || string(got) != string(body) {
		t.Fatalf("local get %q %v", got, err)
	}
	if fake.readCount() != 0 {
		t.Fatalf("local hit still read the origin %d times", fake.readCount())
	}

	// Existing R2 object is not pulled into the hot dir.
	if _, err = client.putRemote(ctx, ClassPhotos, "m/legacy.dat", bytes.NewReader([]byte("legacy")), 6, "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	if client.hot.has(ClassPhotos, "m/legacy.dat") {
		t.Fatal("existing R2 object was copied into the hot tier")
	}
	got, err = client.GetPhotoFile(ctx, "m/legacy.dat", 0, 32)
	if err != nil || string(got) != "legacy" {
		t.Fatalf("fallback get %q %v", got, err)
	}

	// Age the local object and migrate it.
	full, err := client.hot.localPath(ClassPhotos, "m/1.dat")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(full, old, old); err != nil {
		t.Fatal(err)
	}
	meta := filepath.Join(filepath.Dir(full), "1.dat.meta")
	// created_at inside meta still wins unless we rewrite it; force by editing? loadMeta uses CreatedAt.
	// write() set CreatedAt to now. Set diskUsage so retention is 0 (emergency) instead.
	client.hot.diskUsage = func(string) (uint64, uint64, error) { return 95, 100, nil }
	client.hot.now = func() time.Time { return time.Now().Add(time.Hour) }
	if err := client.hot.MigrateOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if client.hot.has(ClassPhotos, "m/1.dat") {
		t.Fatal("local copy survived a successful migrate")
	}
	if !fake.has("tellomi-attachments", "photos/m/1.dat", string(body)) {
		t.Fatal("migrated object missing from origin")
	}
	got, err = client.GetPhotoFile(ctx, "m/1.dat", 0, 32)
	if err != nil || string(got) != string(body) {
		t.Fatalf("post-migrate get %q %v", got, err)
	}

	if err := client.RemoveClassObject(ctx, ClassPhotos, "m/1.dat"); err != nil {
		t.Fatal(err)
	}
	if fake.has("tellomi-attachments", "photos/m/1.dat", string(body)) {
		t.Fatal("purge left the R2 object")
	}
	if client.hot.has(ClassPhotos, "m/1.dat") {
		t.Fatal("purge left a local copy")
	}

	if direct.Load() != 0 {
		t.Fatalf("direct dials %d", direct.Load())
	}
	if connects.Load() == 0 {
		t.Fatal("proxy saw no CONNECT")
	}
	_ = meta
}

func TestHotTierMigrateVerifyMismatchKeepsLocal(t *testing.T) {
	fake := newS3Fake()
	fake.allow("tellomi-attachments", "tellomi-public-media", "tellomi-backups")
	flip := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "photos/m/bad.dat") {
			body := []byte("not-the-original-bytes")
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", "22")
			w.Header().Set("ETag", `"bad"`)
			w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
			w.Write(body)
			return
		}
		fake.ServeHTTP(w, r)
	})
	origin := httptest.NewTLSServer(flip)
	defer origin.Close()
	proxy, _ := connectProxy(t, "tunnel")
	defer proxy.Close()
	var direct atomic.Int32
	tr := tlsTransport(t, origin.Listener.Addr().String(), &direct, 0)
	dir := t.TempDir()
	cfg := hotTestConfig(t, origin, tr, dir, proxy.URL)
	client, err := NewMinioClient(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	body := []byte("original-bytes-ok")
	if _, err = client.PutPhotoFile(ctx, "m/bad.dat", body); err != nil {
		t.Fatal(err)
	}
	full, _ := client.hot.localPath(ClassPhotos, "m/bad.dat")
	old := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(full, old, old)
	client.hot.diskUsage = func(string) (uint64, uint64, error) { return 95, 100, nil }
	if err := client.hot.MigrateOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if !client.hot.has(ClassPhotos, "m/bad.dat") {
		t.Fatal("local copy was deleted after a sha256 mismatch")
	}
	got, err := client.GetPhotoFile(ctx, "m/bad.dat", 0, 32)
	if err != nil || string(got) != string(body) {
		t.Fatalf("local after failed migrate %q %v", got, err)
	}
	if direct.Load() != 0 {
		t.Fatalf("direct dials %d", direct.Load())
	}
}

func TestHotTierConcurrentUploadAndMigrate(t *testing.T) {
	fake := newS3Fake()
	fake.allow("tellomi-attachments", "tellomi-public-media", "tellomi-backups")
	origin := httptest.NewTLSServer(fake)
	defer origin.Close()
	proxy, _ := connectProxy(t, "tunnel")
	defer proxy.Close()
	var direct atomic.Int32
	tr := tlsTransport(t, origin.Listener.Addr().String(), &direct, 0)
	cfg := hotTestConfig(t, origin, tr, t.TempDir(), proxy.URL)
	client, err := NewMinioClient(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	reader, writer := io.Pipe()
	var wg sync.WaitGroup
	wg.Add(2)
	errCh := make(chan error, 2)
	go func() {
		defer wg.Done()
		_, err := client.PutPhotoFileV2(ctx, "m/slow.dat", reader)
		errCh <- err
	}()
	go func() {
		defer wg.Done()
		time.Sleep(20 * time.Millisecond)
		client.hot.diskUsage = func(string) (uint64, uint64, error) { return 95, 100, nil }
		errCh <- client.hot.MigrateOnce(ctx)
	}()
	time.Sleep(30 * time.Millisecond)
	if _, err := io.Copy(writer, bytes.NewReader([]byte("abcdefghijklmnopqrstuvwxyz"))); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := client.GetPhotoFile(ctx, "m/slow.dat", 0, 64)
	if err != nil || string(got) != "abcdefghijklmnopqrstuvwxyz" {
		t.Fatalf("after concurrent migrate %q %v", got, err)
	}
	if direct.Load() != 0 {
		t.Fatalf("direct dials %d", direct.Load())
	}
}

func TestHotTierRetentionShortensWithDiskUsage(t *testing.T) {
	h := defaultHotSettings()
	applyHotDefaults(&h)
	tr := &hotTier{
		maxAge:   h.MaxAge,
		pct7d:    h.UsedPct7d,
		pct1d:    h.UsedPct1d,
		pct12h:   h.UsedPct12h,
		pct1h:    h.UsedPct1h,
		pctEmerg: h.UsedPctEmerg,
	}
	cases := []struct {
		used, total uint64
		want        time.Duration
	}{
		{35, 100, 7 * 24 * time.Hour},
		{60, 100, 24 * time.Hour},
		{74, 100, 24 * time.Hour},
		{75, 100, 12 * time.Hour},
		{84, 100, 12 * time.Hour},
		{85, 100, time.Hour},
		{89, 100, time.Hour},
		{90, 100, 0},
		{99, 100, 0},
	}
	for _, tc := range cases {
		if got := tr.retention(tc.used, tc.total); got != tc.want {
			t.Fatalf("used %d/%d: got %s want %s", tc.used, tc.total, got, tc.want)
		}
	}
}

func TestHotTierRejectingProxyDoesNotDial(t *testing.T) {
	fake := newS3Fake()
	fake.allow("tellomi-attachments", "tellomi-public-media", "tellomi-backups")
	origin := httptest.NewTLSServer(fake)
	defer origin.Close()
	proxy, connects := connectProxy(t, "reject")
	defer proxy.Close()
	var direct atomic.Int32
	tr := tlsTransport(t, origin.Listener.Addr().String(), &direct, 0)
	cfg := hotTestConfig(t, origin, tr, t.TempDir(), proxy.URL)
	_, err := NewMinioClient(&cfg)
	if err == nil {
		t.Fatal("hot tier client started through a rejecting proxy")
	}
	if direct.Load() != 0 {
		t.Fatalf("direct dials %d", direct.Load())
	}
	if connects.Load() == 0 {
		t.Fatal("reject was not sent to the proxy")
	}
	if strings.Contains(strings.ToLower(err.Error()), "direct dial") {
		t.Fatalf("fell back to direct: %v", err)
	}
}

func TestApplyEnvHotTierSettings(t *testing.T) {
	t.Setenv(EnvHotTierEnabled, "true")
	t.Setenv(EnvHotTierDir, "/data/tellomi/media-hot")
	t.Setenv(EnvHotTierMaxAge, "168h")
	t.Setenv(EnvHotTierUsedPct7d, "60")
	t.Setenv(EnvHTTPProxy, "http://172.19.44.222:3128")
	t.Setenv(EnvEndpoint, "https://account.r2.cloudflarestorage.com")
	t.Setenv(EnvAccessKeyID, "ak")
	t.Setenv(EnvSecretAccessKey, "sk")
	t.Setenv(EnvBucketAttachments, "att")
	t.Setenv(EnvBucketPublicMedia, "pub")
	cfg := MinioConfig{}
	if err := cfg.ApplyEnv(); err != nil {
		t.Fatal(err)
	}
	if !cfg.hot.Enabled || cfg.hot.Dir != "/data/tellomi/media-hot" || cfg.hot.MaxAge != 168*time.Hour {
		t.Fatalf("%+v", cfg.hot)
	}
	if err := validateHotTier(cfg.hot, cfg.HTTPProxy); err != nil {
		t.Fatal(err)
	}
}

func TestHotTierDocumentRangeAndVoicePath(t *testing.T) {
	fake := newS3Fake()
	fake.allow("tellomi-attachments", "tellomi-public-media", "tellomi-backups")
	origin := httptest.NewTLSServer(fake)
	defer origin.Close()
	proxy, connects := connectProxy(t, "tunnel")
	defer proxy.Close()
	var direct atomic.Int32
	tr := tlsTransport(t, origin.Listener.Addr().String(), &direct, 0)
	cfg := hotTestConfig(t, origin, tr, t.TempDir(), proxy.URL)
	client, err := NewMinioClient(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	payload := []byte("ogg-voice-and-more-bytes")
	if _, err = client.PutDocumentFile(ctx, "9.dat", bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	got, err := client.GetDocumentFile(ctx, "9.dat", 4, 5)
	if err != nil || string(got) != "voice" {
		t.Fatalf("range %q %v", got, err)
	}
	n, err := client.Stat(ctx, ClassDocuments, "9.dat")
	if err != nil || n != int64(len(payload)) {
		t.Fatalf("stat %d %v", n, err)
	}
	if direct.Load() != 0 {
		t.Fatalf("direct dials %d", direct.Load())
	}
	if connects.Load() == 0 {
		// Startup bucket probes go through the proxy.
		t.Fatal("proxy saw no CONNECT")
	}
}
