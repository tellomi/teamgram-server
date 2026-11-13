package minio_util

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestR2FirstByteBudgetStaysUnderFiveSeconds(t *testing.T) {
	budget := time.Duration(r2GetAttempts)*r2FirstByteTimeout + time.Duration(r2GetAttempts-1)*r2GetRetryGap
	if budget >= 5*time.Second {
		t.Fatalf("first-byte budget %s", budget)
	}
}

func TestSlowR2FirstByteRetriesWithoutStallingFiveSeconds(t *testing.T) {
	t.Setenv("TELLOMI_DFS_CHUNK_CACHE_DIR", t.TempDir())
	fake := newS3Fake()
	fake.allow("tellomi-attachments", "tellomi-public-media", "tellomi-backups")
	var gets atomic.Int32
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && !r.URL.Query().Has("location") && objectPath(r.URL.Path) {
			n := gets.Add(1)
			if n == 1 {
				select {
				case <-r.Context().Done():
				case <-time.After(10 * time.Second):
				}
				return
			}
		}
		fake.ServeHTTP(w, r)
	}))
	t.Cleanup(origin.Close)

	tr := origin.Client().Transport.(*http.Transport).Clone()
	tr.TLSClientConfig = origin.Client().Transport.(*http.Transport).TLSClientConfig
	cfg := objectConfig(origin, tr)
	client, err := NewMinioClient(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const body = "photo-bytes"
	if _, err = client.PutPhotoFile(ctx, "m/1.dat", []byte(body)); err != nil {
		t.Fatal(err)
	}
	// The put is not a GET. Count only the download attempts.
	gets.Store(0)
	start := time.Now()
	got, err := client.GetPhotoFile(ctx, "m/1.dat", 0, 64)
	elapsed := time.Since(start)
	if err != nil || string(got) != body {
		t.Fatalf("get %q %v after %s", got, err, elapsed)
	}
	if elapsed >= 5*time.Second {
		t.Fatalf("slow first byte stalled getFile for %s", elapsed)
	}
	if gets.Load() < 2 {
		t.Fatalf("retries %d, want the hung attempt and a successful one", gets.Load())
	}
}

func TestHotPhotoDoesNotWaitOnHungR2(t *testing.T) {
	t.Setenv("TELLOMI_DFS_CHUNK_CACHE_DIR", t.TempDir())
	fake := newS3Fake()
	fake.allow("tellomi-attachments", "tellomi-public-media", "tellomi-backups")
	var gets atomic.Int32
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && !r.URL.Query().Has("location") && objectPath(r.URL.Path) {
			gets.Add(1)
			select {
			case <-r.Context().Done():
			case <-time.After(10 * time.Second):
			}
			return
		}
		fake.ServeHTTP(w, r)
	}))
	t.Cleanup(origin.Close)
	proxy, _ := connectProxy(t, "tunnel")
	t.Cleanup(proxy.Close)
	tr := origin.Client().Transport.(*http.Transport).Clone()
	tr.TLSClientConfig = origin.Client().Transport.(*http.Transport).TLSClientConfig
	cfg := hotTestConfig(t, origin, tr, t.TempDir(), proxy.URL)
	client, err := NewMinioClient(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const body = "local-photo"
	if _, err = client.PutPhotoFile(ctx, "s/2.dat", []byte(body)); err != nil {
		t.Fatal(err)
	}
	if _, err = client.PutPhotoFile(ctx, "m/2.dat", []byte(body+"-m")); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	got, err := client.GetPhotoFile(ctx, "s/2.dat", 0, 64)
	if err != nil || string(got) != body {
		t.Fatalf("s %q %v", got, err)
	}
	got, err = client.GetPhotoFile(ctx, "m/2.dat", 0, 64)
	elapsed := time.Since(start)
	if err != nil || string(got) != body+"-m" {
		t.Fatalf("m %q %v", got, err)
	}
	if elapsed >= 500*time.Millisecond {
		t.Fatalf("hot get took %s", elapsed)
	}
	if gets.Load() != 0 {
		t.Fatalf("hot hit still called R2 %d times", gets.Load())
	}
}

func objectPath(p string) bool {
	n := 0
	for _, c := range p {
		if c == '/' {
			n++
		}
	}
	return n >= 2
}

func TestColdHandshakeGETSucceedsOnFirstAttempt(t *testing.T) {
	t.Setenv("TELLOMI_DFS_CHUNK_CACHE_DIR", t.TempDir())
	fake := newS3Fake()
	fake.allow("tellomi-attachments", "tellomi-public-media", "tellomi-backups")
	var gets atomic.Int32
	// 1.6s of connection setup plus 0.7s before headers is about 2.3s.
	// A 2s clock that starts at RoundTrip cancels this. A clock that
	// starts after the request is written does not.
	const setup = 1600 * time.Millisecond
	const headerDelay = 700 * time.Millisecond
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && !r.URL.Query().Has("location") && objectPath(r.URL.Path) {
			gets.Add(1)
			timer := time.NewTimer(headerDelay)
			defer timer.Stop()
			select {
			case <-r.Context().Done():
				return
			case <-timer.C:
			}
		}
		fake.ServeHTTP(w, r)
	}))
	t.Cleanup(origin.Close)

	tr := origin.Client().Transport.(*http.Transport).Clone()
	tr.TLSClientConfig = origin.Client().Transport.(*http.Transport).TLSClientConfig
	prev := tr.DialContext
	if prev == nil {
		prev = (&net.Dialer{Timeout: 5 * time.Second}).DialContext
	}
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		timer := time.NewTimer(setup)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
		return prev(ctx, network, addr)
	}
	cfg := objectConfig(origin, tr)
	client, err := NewMinioClient(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const body = "cold-photo"
	if _, err = client.PutPhotoFile(ctx, "m/cold.dat", []byte(body)); err != nil {
		t.Fatal(err)
	}
	// Drop the pooled connection so the download pays for setup again.
	// A reused socket would hide the handshake the timer must ignore.
	tr.CloseIdleConnections()
	gets.Store(0)
	start := time.Now()
	got, err := client.GetPhotoFile(ctx, "m/cold.dat", 0, 64)
	elapsed := time.Since(start)
	if err != nil || string(got) != body {
		t.Fatalf("get %q %v after %s", got, err, elapsed)
	}
	if gets.Load() != 1 {
		t.Fatalf("attempts %d, want one; a handshake must not burn the header budget", gets.Load())
	}
	if elapsed < setup {
		t.Fatalf("get took %s, connection setup was skipped", elapsed)
	}
	if elapsed >= 5*time.Second {
		t.Fatalf("cold get took %s", elapsed)
	}
}

func TestRangeReadSlowBodyIsNotCutOff(t *testing.T) {
	t.Setenv("TELLOMI_DFS_CHUNK_CACHE_DIR", t.TempDir())
	fake := newS3Fake()
	fake.allow("tellomi-attachments", "tellomi-public-media", "tellomi-backups")
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && !r.URL.Query().Has("location") && objectPath(r.URL.Path) {
			w = &trickleBody{ResponseWriter: w, delay: 3 * time.Second}
		}
		fake.ServeHTTP(w, r)
	}))
	t.Cleanup(origin.Close)
	tr := origin.Client().Transport.(*http.Transport).Clone()
	tr.TLSClientConfig = origin.Client().Transport.(*http.Transport).TLSClientConfig
	cfg := objectConfig(origin, tr)
	client, err := NewMinioClient(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	body := bytes.Repeat([]byte("v"), 64*1024)
	if _, err = client.PutVideoFile(ctx, "clip.dat", body); err != nil {
		t.Fatal(err)
	}
	const offset, limit = 100, int32(200)
	start := time.Now()
	got, err := client.GetVideoFile(ctx, "clip.dat", offset, limit)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("range get after %s: %v", elapsed, err)
	}
	if !bytes.Equal(got, body[offset:offset+limit]) {
		t.Fatalf("range len %d, want %d", len(got), limit)
	}
	if elapsed < 2*time.Second {
		t.Fatalf("body was not slow (%s); the timer was not given a chance to cancel it", elapsed)
	}
}

func TestHungGetPhotoAndVideoFinishUnderFiveSeconds(t *testing.T) {
	t.Setenv("TELLOMI_DFS_CHUNK_CACHE_DIR", t.TempDir())
	fake := newS3Fake()
	fake.allow("tellomi-attachments", "tellomi-public-media", "tellomi-backups")
	var gets atomic.Int32
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && !r.URL.Query().Has("location") && objectPath(r.URL.Path) {
			gets.Add(1)
			select {
			case <-r.Context().Done():
			case <-time.After(30 * time.Second):
			}
			return
		}
		fake.ServeHTTP(w, r)
	}))
	t.Cleanup(origin.Close)
	tr := origin.Client().Transport.(*http.Transport).Clone()
	tr.TLSClientConfig = origin.Client().Transport.(*http.Transport).TLSClientConfig
	cfg := objectConfig(origin, tr)
	client, err := NewMinioClient(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = client.PutPhotoFile(ctx, "m/hung.dat", []byte("photo")); err != nil {
		t.Fatal(err)
	}
	if _, err = client.PutVideoFile(ctx, "hung.dat", []byte("video-bytes")); err != nil {
		t.Fatal(err)
	}
	check := func(name string, fn func() error) {
		t.Helper()
		before := gets.Load()
		start := time.Now()
		err := fn()
		elapsed := time.Since(start)
		if err == nil {
			t.Fatalf("%s succeeded against a hung origin", name)
		}
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			t.Fatalf("%s err %v, want the header budget to stop minio retries", name, err)
		}
		if elapsed >= 5*time.Second {
			t.Fatalf("%s took %s", name, elapsed)
		}
		if n := gets.Load() - before; n > int32(r2GetAttempts) {
			t.Fatalf("%s attempts %d, minio-go multiplied the wait", name, n)
		}
	}
	check("GetPhotoFile", func() error {
		_, err := client.GetPhotoFile(ctx, "m/hung.dat", 0, 32)
		return err
	})
	check("GetVideoFile", func() error {
		_, err := client.GetVideoFile(ctx, "hung.dat", 0, 32)
		return err
	})
}

func TestHotTierMigrateVerifyWithFirstByteTimeout(t *testing.T) {
	t.Setenv("TELLOMI_DFS_CHUNK_CACHE_DIR", t.TempDir())
	fake := newS3Fake()
	fake.allow("tellomi-attachments", "tellomi-public-media", "tellomi-backups")
	origin := httptest.NewTLSServer(fake)
	t.Cleanup(origin.Close)
	proxy, _ := connectProxy(t, "tunnel")
	t.Cleanup(proxy.Close)
	tr := origin.Client().Transport.(*http.Transport).Clone()
	tr.TLSClientConfig = origin.Client().Transport.(*http.Transport).TLSClientConfig
	cfg := hotTestConfig(t, origin, tr, t.TempDir(), proxy.URL)
	client, err := NewMinioClient(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	body := []byte("migrate-me")
	if _, err = client.PutPhotoFile(ctx, "m/move.dat", body); err != nil {
		t.Fatal(err)
	}
	if fake.has("tellomi-attachments", "photos/m/move.dat", string(body)) {
		t.Fatal("put waited on R2")
	}
	full, err := client.hot.localPath(ClassPhotos, "m/move.dat")
	if err != nil {
		t.Fatal(err)
	}
	client.hot.diskUsage = func(string) (uint64, uint64, error) { return 95, 100, nil }
	client.hot.now = func() time.Time { return time.Now().Add(time.Hour) }
	if err := os.Chtimes(full, time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := client.hot.MigrateOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if client.hot.has(ClassPhotos, "m/move.dat") {
		t.Fatal("local copy survived verify")
	}
	if !fake.has("tellomi-attachments", "photos/m/move.dat", string(body)) {
		t.Fatal("verified object missing from origin")
	}
}

// trickleBody flushes response headers, then waits before the body. The
// first-byte timer must already be stopped or this wait cancels the read.
type trickleBody struct {
	http.ResponseWriter
	delay time.Duration
	once  sync.Once
}

func (s *trickleBody) Write(p []byte) (int, error) {
	s.once.Do(func() {
		if f, ok := s.ResponseWriter.(http.Flusher); ok {
			f.Flush()
		}
		time.Sleep(s.delay)
	})
	return s.ResponseWriter.Write(p)
}
