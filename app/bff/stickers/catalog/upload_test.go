package catalog

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/teamgram/teamgram-server/app/service/dfs/objectstore"
)

func TestUploadAndGetUseTheSameObject(t *testing.T) {
	fake := newStickerS3()
	fake.allow("tellomi-prod-public-media", "tellomi-prod-attachments")
	srv := httptest.NewTLSServer(fake)
	defer srv.Close()
	transport, err := minio.DefaultTransport(true)
	if err != nil {
		t.Fatal(err)
	}
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	objectstore.SetRoundTripperForTest(transport)
	defer objectstore.SetRoundTripperForTest(nil)

	t.Setenv("TELLOMI_S3_ENDPOINT", srv.URL)
	t.Setenv("TELLOMI_S3_REGION", "auto")
	t.Setenv("TELLOMI_S3_ACCESS_KEY_ID", "ak")
	t.Setenv("TELLOMI_S3_SECRET_ACCESS_KEY", "sk")
	t.Setenv("TELLOMI_S3_FORCE_PATH_STYLE", "true")
	t.Setenv("TELLOMI_S3_USE_SSL", "")
	t.Setenv("TELLOMI_S3_BUCKET_ATTACHMENTS", "tellomi-prod-attachments")
	t.Setenv("TELLOMI_S3_BUCKET_PUBLIC_MEDIA", "tellomi-prod-public-media")
	t.Setenv("TELLOMI_S3_BUCKET_BACKUPS", "")

	const (
		stickerPath = "42.dat"
		emojiPath   = "43.dat"
	)
	webp, err := assetFS.ReadFile("assets/twemoji/1f600.webp")
	if err != nil {
		t.Fatal(err)
	}
	tgs, err := assetFS.ReadFile("assets/noto/1f600.tgs")
	if err != nil {
		t.Fatal(err)
	}
	packs := []*Pack{{
		Items: []*Item{
			{StorageClass: "stickers", ObjectPath: stickerPath, AssetPath: "assets/twemoji/1f600.webp", MIME: "image/webp"},
			{StorageClass: "emoji", ObjectPath: emojiPath, AssetPath: "assets/noto/1f600.tgs", MIME: "application/x-tgsticker"},
			// A second pack row for the same emoji object must not be a second key.
			{StorageClass: "emoji", ObjectPath: emojiPath, AssetPath: "assets/noto/1f600.tgs", MIME: "application/x-tgsticker"},
		},
	}}
	if err = uploadAssets(context.Background(), packs); err != nil {
		t.Fatal(err)
	}
	if !fake.has("tellomi-prod-public-media", "stickers/"+stickerPath, webp) {
		t.Fatalf("sticker key missing, objects=%v", fake.keys("tellomi-prod-public-media"))
	}
	if !fake.has("tellomi-prod-public-media", "emoji/"+emojiPath, tgs) {
		t.Fatalf("emoji key missing, objects=%v", fake.keys("tellomi-prod-public-media"))
	}
	if len(fake.keys("tellomi-prod-attachments")) != 0 {
		t.Fatalf("stickers were written to the attachments bucket: %v", fake.keys("tellomi-prod-attachments"))
	}

	client, err := objectstore.Open()
	if err != nil {
		t.Fatal(err)
	}
	if client == nil {
		t.Fatal("expected a client")
	}
	got, err := client.Get(context.Background(), "stickers", stickerPath, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, webp) {
		t.Fatalf("GetStickerFile returned %d bytes, want %d", len(got), len(webp))
	}
	got, err = client.Get(context.Background(), "emoji", emojiPath, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, tgs) {
		t.Fatalf("GetEmojiFile returned %d bytes, want %d", len(got), len(tgs))
	}
}

func TestUploadSkipsWhenStorageIsUnset(t *testing.T) {
	for _, name := range []string{
		"TELLOMI_S3_ENDPOINT",
		"TELLOMI_S3_ACCESS_KEY_ID",
		"TELLOMI_S3_SECRET_ACCESS_KEY",
		"TELLOMI_S3_BUCKET_ATTACHMENTS",
		"TELLOMI_S3_BUCKET_PUBLIC_MEDIA",
	} {
		t.Setenv(name, "")
	}
	if err := uploadAssets(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

type stickerS3 struct {
	mu      sync.Mutex
	buckets map[string]bool
	objects map[string]map[string][]byte
}

func newStickerS3() *stickerS3 {
	return &stickerS3{
		buckets: map[string]bool{},
		objects: map[string]map[string][]byte{},
	}
}

func (s *stickerS3) allow(names ...string) {
	for _, name := range names {
		s.buckets[name] = true
		s.objects[name] = map[string][]byte{}
	}
}

func (s *stickerS3) has(bucket, key string, body []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	got, ok := s.objects[bucket][key]
	return ok && bytes.Equal(got, body)
}

func (s *stickerS3) keys(bucket string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for key := range s.objects[bucket] {
		out = append(out, key)
	}
	return out
}

func (s *stickerS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bucket, key := splitS3Path(r.URL.Path)
	if bucket == "" || !s.buckets[bucket] {
		http.Error(w, "NoSuchBucket", http.StatusNotFound)
		return
	}
	switch {
	case r.Method == http.MethodHead && key == "":
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodGet && key == "" && r.URL.Query().Has("location"):
		fmt.Fprint(w, "<LocationConstraint>auto</LocationConstraint>")
	case r.Method == http.MethodPut && key != "":
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.objects[bucket][key] = body
		s.mu.Unlock()
		w.Header().Set("ETag", `"put"`)
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodGet && key != "":
		s.mu.Lock()
		body, ok := s.objects[bucket][key]
		s.mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, "<Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message><Key>%s</Key></Error>", key)
			return
		}
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("ETag", `"obj"`)
		start, end := 0, len(body)-1
		if rng := r.Header.Get("Range"); strings.HasPrefix(rng, "bytes=") {
			fmt.Sscanf(strings.TrimPrefix(rng, "bytes="), "%d-%d", &start, &end)
			if end >= len(body) {
				end = len(body) - 1
			}
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(body[start : end+1])
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		_, _ = w.Write(body)
	default:
		http.Error(w, "unexpected "+r.Method+" "+r.URL.RequestURI(), http.StatusNotImplemented)
	}
}

func splitS3Path(p string) (bucket, key string) {
	p = strings.TrimPrefix(p, "/")
	bucket, key, _ = strings.Cut(p, "/")
	return bucket, key
}
