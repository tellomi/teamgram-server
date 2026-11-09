package catalog

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
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
			// A pack imported with -dir has no embedded asset and must not fail the seed pass.
			{StorageClass: "stickers", ObjectPath: "44.dat", MIME: "image/webp"},
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
	centerBody, centerMIME := reactionSlotBytes("1f44d", "center", true)
	if centerMIME != "application/x-tgsticker" || !tgsCanvas512(centerBody) {
		t.Fatal("center slot was not a 512 tgs")
	}
	centerKey := "emoji/" + fmt.Sprint(reactionSlotID("1f44d", "center")) + ".dat"
	legacyKey := "emoji/" + fmt.Sprint(legacyCenterID("1f44d")) + ".dat"
	if !fake.has("tellomi-prod-public-media", centerKey, centerBody) || !fake.has("tellomi-prod-public-media", legacyKey, centerBody) {
		t.Fatal("reaction reload did not write the inset center icon")
	}
	for _, key := range fake.keys("tellomi-prod-public-media") {
		if key == "stickers/44.dat" {
			t.Fatal("seed pass uploaded an imported item with no embedded asset")
		}
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

func TestUploadSkipsUnchangedObjects(t *testing.T) {
	fake, client := stickerClient(t)
	packs := []*Pack{{
		Items: []*Item{
			{StorageClass: "stickers", ObjectPath: "42.dat", AssetPath: "assets/twemoji/1f600.webp", MIME: "image/webp"},
		},
	}}
	if err := uploadChanged(context.Background(), client, mustSeed(t, packs)); err != nil {
		t.Fatal(err)
	}
	first := fake.puts.Load()
	if first == 0 {
		t.Fatal("expected the first pass to upload")
	}
	if err := uploadChanged(context.Background(), client, mustSeed(t, packs)); err != nil {
		t.Fatal(err)
	}
	if got := fake.puts.Load(); got != first {
		t.Fatalf("second pass uploaded %d more objects (%d -> %d)", got-first, first, got)
	}
	// A different body under the same key must upload again.
	objs := mustSeed(t, packs)
	for i := range objs {
		if objs[i].path == "42.dat" {
			objs[i].body = append([]byte(nil), objs[i].body...)
			objs[i].body = append(objs[i].body, 1)
		}
	}
	if err := uploadChanged(context.Background(), client, objs); err != nil {
		t.Fatal(err)
	}
	if got := fake.puts.Load(); got != first+1 {
		t.Fatalf("changed object puts %d, want %d", got, first+1)
	}
}

func TestUploadPutsAreBounded(t *testing.T) {
	store := &delayStore{delay: 40 * time.Millisecond}
	var objs []seedObject
	for i := 0; i < seedPutConcurrency; i++ {
		objs = append(objs, seedObject{class: "emoji", path: fmt.Sprintf("%d.dat", i), mime: "image/webp", body: []byte{byte(i), 1, 2, 3}})
	}
	start := time.Now()
	if err := uploadChanged(context.Background(), store, objs); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if store.max < 2 {
		t.Fatalf("peak concurrency %d", store.max)
	}
	if store.max > seedPutConcurrency {
		t.Fatalf("peak concurrency %d over the cap %d", store.max, seedPutConcurrency)
	}
	// Sequential would be concurrency*delay. Parallel is about one delay.
	if elapsed > store.delay*3 {
		t.Fatalf("took %s, want near %s", elapsed, store.delay)
	}
	t.Logf("%d puts peak %d in %s", len(objs), store.max, elapsed)
}

func mustSeed(t *testing.T, packs []*Pack) []seedObject {
	t.Helper()
	objs, err := seedObjects(packs)
	if err != nil {
		t.Fatal(err)
	}
	return objs
}

type delayStore struct {
	mu      sync.Mutex
	current int
	max     int
	delay   time.Duration
}

func (s *delayStore) List(context.Context, string) (map[string]objectstore.Head, error) {
	return map[string]objectstore.Head{}, nil
}

func (s *delayStore) Put(ctx context.Context, _, _, _ string, _ []byte) error {
	s.mu.Lock()
	s.current++
	if s.current > s.max {
		s.max = s.current
	}
	s.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(s.delay):
	}
	s.mu.Lock()
	s.current--
	s.mu.Unlock()
	return nil
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
	puts    atomic.Int32
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
	case r.Method == http.MethodHead && key != "":
		s.mu.Lock()
		body, ok := s.objects[bucket][key]
		s.mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, "<Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message><Key>%s</Key></Error>", key)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("ETag", md5ETag(body))
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodGet && key == "" && r.URL.Query().Get("list-type") == "2":
		s.serveList(w, bucket, r.URL.Query().Get("prefix"))
	case r.Method == http.MethodGet && key == "" && r.URL.Query().Has("location"):
		fmt.Fprint(w, "<LocationConstraint>auto</LocationConstraint>")
	case r.Method == http.MethodPut && key != "":
		body, _ := io.ReadAll(r.Body)
		s.puts.Add(1)
		s.mu.Lock()
		s.objects[bucket][key] = body
		s.mu.Unlock()
		w.Header().Set("ETag", md5ETag(body))
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
		w.Header().Set("ETag", md5ETag(body))
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

func md5ETag(body []byte) string {
	sum := md5.Sum(body)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

type listBucketResult struct {
	XMLName     xml.Name     `xml:"ListBucketResult"`
	Xmlns       string       `xml:"xmlns,attr"`
	Name        string       `xml:"Name"`
	Prefix      string       `xml:"Prefix"`
	KeyCount    int          `xml:"KeyCount"`
	MaxKeys     int          `xml:"MaxKeys"`
	IsTruncated bool         `xml:"IsTruncated"`
	Contents    []listObject `xml:"Contents"`
}

type listObject struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int    `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
}

func (s *stickerS3) serveList(w http.ResponseWriter, bucket, prefix string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := listBucketResult{
		Xmlns:    "http://s3.amazonaws.com/doc/2006-03-01/",
		Name:     bucket,
		Prefix:   prefix,
		MaxKeys:  1000,
		Contents: []listObject{},
	}
	for key, body := range s.objects[bucket] {
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			continue
		}
		res.Contents = append(res.Contents, listObject{
			Key:          key,
			LastModified: "2020-01-01T00:00:00.000Z",
			ETag:         md5ETag(body),
			Size:         len(body),
			StorageClass: "STANDARD",
		})
	}
	res.KeyCount = len(res.Contents)
	w.Header().Set("Content-Type", "application/xml")
	enc := xml.NewEncoder(w)
	if err := enc.Encode(res); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func stickerClient(t *testing.T) (*stickerS3, *objectstore.Client) {
	t.Helper()
	fake := newStickerS3()
	fake.allow("tellomi-prod-public-media", "tellomi-prod-attachments")
	srv := httptest.NewTLSServer(fake)
	t.Cleanup(srv.Close)
	transport, err := minio.DefaultTransport(true)
	if err != nil {
		t.Fatal(err)
	}
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	objectstore.SetRoundTripperForTest(transport)
	t.Cleanup(func() { objectstore.SetRoundTripperForTest(nil) })
	t.Setenv("TELLOMI_S3_ENDPOINT", srv.URL)
	t.Setenv("TELLOMI_S3_REGION", "auto")
	t.Setenv("TELLOMI_S3_ACCESS_KEY_ID", "ak")
	t.Setenv("TELLOMI_S3_SECRET_ACCESS_KEY", "sk")
	t.Setenv("TELLOMI_S3_FORCE_PATH_STYLE", "true")
	t.Setenv("TELLOMI_S3_USE_SSL", "")
	t.Setenv("TELLOMI_S3_BUCKET_ATTACHMENTS", "tellomi-prod-attachments")
	t.Setenv("TELLOMI_S3_BUCKET_PUBLIC_MEDIA", "tellomi-prod-public-media")
	t.Setenv("TELLOMI_S3_BUCKET_BACKUPS", "")
	client, err := objectstore.Open()
	if err != nil {
		t.Fatal(err)
	}
	if client == nil {
		t.Fatal("expected a client")
	}
	return fake, client
}

func splitS3Path(p string) (bucket, key string) {
	p = strings.TrimPrefix(p, "/")
	bucket, key, _ = strings.Cut(p, "/")
	return bucket, key
}
