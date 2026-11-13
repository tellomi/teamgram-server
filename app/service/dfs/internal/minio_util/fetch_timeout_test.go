package minio_util

import (
	"context"
	"net/http"
	"net/http/httptest"
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
