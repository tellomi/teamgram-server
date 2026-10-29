package minio_util

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
)

func TestApplyEnvDefaultsAndR2Endpoint(t *testing.T) {
	t.Setenv(EnvEndpoint, "https://account.r2.cloudflarestorage.com")
	t.Setenv(EnvRegion, "")
	t.Setenv(EnvAccessKeyID, "ak")
	t.Setenv(EnvSecretAccessKey, "sk")
	t.Setenv(EnvForcePathStyle, "")
	t.Setenv(EnvUseSSL, "")
	t.Setenv(EnvBucketAttachments, "tellomi-attachments")
	t.Setenv(EnvBucketPublicMedia, "tellomi-public-media")
	t.Setenv(EnvBucketBackups, "")
	t.Setenv(EnvPublicMediaBaseURL, "https://media.tellomi.app/")
	t.Setenv(EnvAttachmentsBaseURL, "https://cdn3.tellomi.app")

	cfg := MinioConfig{UseSSL: false}
	if err := cfg.ApplyEnv(); err != nil {
		t.Fatal(err)
	}
	if cfg.Endpoint != "account.r2.cloudflarestorage.com" {
		t.Fatalf("endpoint %q", cfg.Endpoint)
	}
	if !cfg.UseSSL {
		t.Fatal("https endpoint should enable SSL when TELLOMI_S3_USE_SSL is unset")
	}
	if cfg.Region != "auto" {
		t.Fatalf("region %q, want auto", cfg.Region)
	}
	if !cfg.ForcePathStyle || cfg.pathStyleDisabled {
		t.Fatalf("path style forced=%v disabled=%v", cfg.ForcePathStyle, cfg.pathStyleDisabled)
	}
	if cfg.Bucket.Backups != "" {
		t.Fatalf("empty backups env should clear the bucket, got %q", cfg.Bucket.Backups)
	}
	if cfg.PublicMediaBaseURL != "https://media.tellomi.app" || cfg.AttachmentsBaseURL != "https://cdn3.tellomi.app" {
		t.Fatalf("base urls %q %q", cfg.PublicMediaBaseURL, cfg.AttachmentsBaseURL)
	}
}

func TestApplyEnvRejectsBadBool(t *testing.T) {
	t.Setenv(EnvEndpoint, "http://127.0.0.1:9000")
	t.Setenv(EnvForcePathStyle, "maybe")
	cfg := MinioConfig{}
	err := cfg.ApplyEnv()
	if err == nil || !strings.Contains(err.Error(), EnvForcePathStyle) {
		t.Fatalf("err = %v", err)
	}
}

func TestLocationPrefixes(t *testing.T) {
	cfg := &MinioConfig{}
	cfg.Bucket.Attachments = "tellomi-attachments"
	cfg.Bucket.PublicMedia = "tellomi-public-media"
	cfg.Bucket.Backups = "tellomi-backups"

	cases := []struct {
		class, path, bucket, key string
	}{
		{ClassDocuments, "12.dat", "tellomi-attachments", "documents/12.dat"},
		{ClassPhotos, "m/9.dat", "tellomi-attachments", "photos/m/9.dat"},
		{ClassVideos, "v/9.dat", "tellomi-attachments", "videos/v/9.dat"},
		{ClassEncryptedFiles, "3.dat", "tellomi-attachments", "encryptedfiles/3.dat"},
		{ClassAvatars, "a/9.dat", "tellomi-public-media", "avatars/a/9.dat"},
		{ClassStickers, "m/4.dat", "tellomi-public-media", "stickers/m/4.dat"},
		{ClassEmoji, "e/1.webp", "tellomi-public-media", "emoji/e/1.webp"},
		{ClassWallpapers, "8.dat", "tellomi-public-media", "wallpapers/8.dat"},
		{ClassBackups, "nightly/db.sql", "tellomi-backups", "nightly/db.sql"},
	}
	for _, tc := range cases {
		bucket, key, err := resolveLocation(cfg, tc.class, tc.path)
		if err != nil {
			t.Fatalf("%s: %v", tc.class, err)
		}
		if bucket != tc.bucket || key != tc.key {
			t.Fatalf("%s -> %s %s, want %s %s", tc.class, bucket, key, tc.bucket, tc.key)
		}
	}
}

func TestLegacyBucketsHaveNoPrefix(t *testing.T) {
	cfg := &MinioConfig{}
	cfg.Bucket.Documents = "documents"
	cfg.Bucket.Photos = "photos"
	cfg.Bucket.Videos = "videos"
	cfg.Bucket.EncryptedFiles = "encryptedfiles"
	bucket, key, err := resolveLocation(cfg, ClassPhotos, "0/1.dat")
	if err != nil {
		t.Fatal(err)
	}
	if bucket != "photos" || key != "0/1.dat" {
		t.Fatalf("legacy photo location %s %s", bucket, key)
	}
	bucket, key, err = resolveLocation(cfg, ClassAvatars, "a/1.dat")
	if err != nil {
		t.Fatal(err)
	}
	if bucket != "photos" || key != "avatars/a/1.dat" {
		t.Fatalf("legacy avatar location %s %s", bucket, key)
	}
}

func TestPrefixModeRequiresPublicBucket(t *testing.T) {
	cfg := MinioConfig{
		Endpoint:        "127.0.0.1:9",
		AccessKeyID:     "minio",
		SecretAccessKey: "miniostorage",
		Region:          "auto",
	}
	cfg.Bucket.Attachments = "tellomi-attachments"
	_, err := NewMinioClient(&cfg)
	if err == nil || !strings.Contains(err.Error(), EnvBucketPublicMedia) {
		t.Fatalf("err = %v", err)
	}
}

func TestMissingBucketIsAClearError(t *testing.T) {
	fake := newS3Fake()
	fake.allow("tellomi-attachments")
	srv, cfg := fake.server(t)
	defer srv.Close()
	cfg.Bucket.PublicMedia = "tellomi-public-media"
	_, err := NewMinioClient(&cfg)
	if err == nil {
		t.Fatal("expected missing public bucket")
	}
	var missing *BucketError
	if !asBucket(err, &missing) || !missing.Missing || missing.Bucket != "tellomi-public-media" {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "MakeBucket") || !strings.Contains(err.Error(), "ListBuckets") {
		t.Fatalf("error should say buckets are not created: %v", err)
	}
	if len(fake.forbidden) != 0 {
		t.Fatalf("forbidden calls %+v", fake.forbidden)
	}
}

func TestPrecreatedBucketsRoundTrip(t *testing.T) {
	fake := newS3Fake()
	fake.allow("tellomi-attachments", "tellomi-public-media", "tellomi-backups")
	srv, cfg := fake.server(t)
	defer srv.Close()

	client, err := NewMinioClient(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if _, err := client.PutPhotoFile(ctx, "m/7.dat", []byte("story-photo")); err != nil {
		t.Fatal(err)
	}
	got, err := client.GetPhotoFile(ctx, "m/7.dat", 0, 64)
	if err != nil || string(got) != "story-photo" {
		t.Fatalf("photo %q %v", got, err)
	}
	part, err := client.GetPhotoFile(ctx, "m/7.dat", 6, 5)
	if err != nil || string(part) != "photo" {
		t.Fatalf("range %q %v", part, err)
	}

	if _, err := client.PutDocumentFile(ctx, "voice.dat", bytes.NewReader([]byte("voice-note"))); err != nil {
		t.Fatal(err)
	}
	got, err = client.GetDocumentFile(ctx, "voice.dat", 0, 32)
	if err != nil || string(got) != "voice-note" {
		t.Fatalf("document %q %v", got, err)
	}

	if _, err := client.PutEncryptedFile(ctx, "secret.dat", bytes.NewReader([]byte("secret"))); err != nil {
		t.Fatal(err)
	}
	got, err = client.GetEncryptedFile(ctx, "secret.dat", 0, 16)
	if err != nil || string(got) != "secret" {
		t.Fatalf("encrypted %q %v", got, err)
	}

	if _, err := client.PutVideoFile(ctx, "v/7.dat", []byte("profile-video")); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PutAvatarFile(ctx, "a/7.dat", []byte("avatar")); err != nil {
		t.Fatal(err)
	}
	got, err = client.GetAvatarFile(ctx, "a/7.dat", 0, 16)
	if err != nil || string(got) != "avatar" {
		t.Fatalf("avatar %q %v", got, err)
	}
	if _, err := client.PutStickerBytes(ctx, "m/1.dat", []byte("sticker")); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PutEmojiBytes(ctx, "e/1.webp", []byte("emoji")); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PutWallpaperBytes(ctx, "9.dat", []byte("wall")); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PutBackup(ctx, "nightly/db.sql", bytes.NewReader([]byte("backup")), 6); err != nil {
		t.Fatal(err)
	}
	got, err = client.GetBackup(ctx, "nightly/db.sql", 0, 16)
	if err != nil || string(got) != "backup" {
		t.Fatalf("backup %q %v", got, err)
	}

	photoURL, ok := client.PublicMediaURL(ClassAvatars, "a/7.dat")
	if !ok || photoURL != "https://media.tellomi.app/avatars/a/7.dat" {
		t.Fatalf("public url %q ok=%v", photoURL, ok)
	}
	fileURL, ok := client.AttachmentsURL(ClassDocuments, "voice.dat")
	if !ok || fileURL != "https://cdn3.tellomi.app/documents/voice.dat" {
		t.Fatalf("attachment url %q ok=%v", fileURL, ok)
	}

	if !fake.has("tellomi-attachments", "photos/m/7.dat", "story-photo") {
		t.Fatalf("photo key missing: %#v", fake.objects["tellomi-attachments"])
	}
	if !fake.has("tellomi-attachments", "documents/voice.dat", "voice-note") {
		t.Fatalf("document key missing: %#v", fake.objects["tellomi-attachments"])
	}
	if !fake.has("tellomi-public-media", "avatars/a/7.dat", "avatar") {
		t.Fatalf("avatar key missing: %#v", fake.objects["tellomi-public-media"])
	}
	if !fake.has("tellomi-public-media", "stickers/m/1.dat", "sticker") ||
		!fake.has("tellomi-public-media", "emoji/e/1.webp", "emoji") ||
		!fake.has("tellomi-public-media", "wallpapers/9.dat", "wall") {
		t.Fatalf("public keys %#v", fake.objects["tellomi-public-media"])
	}
	if len(fake.forbidden) != 0 {
		t.Fatalf("client called a forbidden API: %+v", fake.forbidden)
	}
}

func TestGuardBlocksListAndMake(t *testing.T) {
	g := guardTransport{}
	list, err := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	if err != nil {
		t.Fatal(err)
	}
	makeBucket, err := http.NewRequest(http.MethodPut, "https://example.test/tellomi-attachments", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, req := range []*http.Request{list, makeBucket} {
		_, err := g.RoundTrip(req)
		if err == nil {
			t.Fatalf("%s %s was allowed", req.Method, req.URL.Path)
		}
	}
	head, err := http.NewRequest(http.MethodHead, "https://example.test/tellomi-attachments", nil)
	if err != nil {
		t.Fatal(err)
	}
	if isListBucketsRequest(head) || isMakeBucketRequest(head) {
		t.Fatal("HeadBucket was classified as list or create")
	}
	put, err := http.NewRequest(http.MethodPut, "https://example.test/tellomi-attachments/photos/1.dat", nil)
	if err != nil {
		t.Fatal(err)
	}
	if isMakeBucketRequest(put) || isListBucketsRequest(put) {
		t.Fatal("PutObject was classified as list or create")
	}
}

func TestSourceDoesNotCallMakeOrList(t *testing.T) {
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range matches {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		if strings.Contains(text, ".MakeBucket(") || strings.Contains(text, ".ListBuckets(") {
			t.Fatalf("%s calls MakeBucket or ListBuckets", name)
		}
	}
}

func asBucket(err error, target **BucketError) bool {
	for err != nil {
		if be, ok := err.(*BucketError); ok {
			*target = be
			return true
		}
		unwrap, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrap.Unwrap()
	}
	return false
}

type s3fake struct {
	mu        sync.Mutex
	buckets   map[string]bool
	objects   map[string]map[string][]byte
	uploads   map[string]*upload
	forbidden []string
}

type upload struct {
	bucket string
	key    string
	parts  map[int][]byte
}

func newS3Fake() *s3fake {
	return &s3fake{
		buckets: map[string]bool{},
		objects: map[string]map[string][]byte{},
		uploads: map[string]*upload{},
	}
}

func (s *s3fake) allow(names ...string) {
	for _, name := range names {
		s.buckets[name] = true
		s.objects[name] = map[string][]byte{}
	}
}

func (s *s3fake) has(bucket, key, body string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	got, ok := s.objects[bucket][key]
	return ok && string(got) == body
}

func (s *s3fake) server(t *testing.T) (*httptest.Server, MinioConfig) {
	t.Helper()
	srv := httptest.NewTLSServer(s)
	tr, err := minio.DefaultTransport(true)
	if err != nil {
		t.Fatal(err)
	}
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	cfg := MinioConfig{
		Endpoint:            srv.Listener.Addr().String(),
		AccessKeyID:         "minio",
		SecretAccessKey:     "miniostorage",
		UseSSL:              true,
		Region:              "auto",
		ForcePathStyle:      true,
		BucketProbeAttempts: 1,
		transport:           tr,
		PublicMediaBaseURL:  "https://media.tellomi.app",
		AttachmentsBaseURL:  "https://cdn3.tellomi.app",
	}
	cfg.Bucket.Attachments = "tellomi-attachments"
	cfg.Bucket.PublicMedia = "tellomi-public-media"
	cfg.Bucket.Backups = "tellomi-backups"
	return srv, cfg
}

func (s *s3fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if isListBucketsRequest(r) || isMakeBucketRequest(r) {
		s.mu.Lock()
		s.forbidden = append(s.forbidden, r.Method+" "+r.URL.RequestURI())
		s.mu.Unlock()
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	bucket, key := splitPath(r.URL.Path)
	if bucket == "" || !s.buckets[bucket] {
		writeS3Error(w, http.StatusNotFound, "NoSuchBucket", bucket, key)
		return
	}
	q := r.URL.Query()
	switch {
	case r.Method == http.MethodHead && key == "":
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodGet && key == "" && q.Has("location"):
		fmt.Fprint(w, "<LocationConstraint>auto</LocationConstraint>")
	case r.Method == http.MethodPost && q.Has("uploads"):
		s.initiate(w, bucket, key)
	case r.Method == http.MethodPut && q.Get("uploadId") != "" && q.Get("partNumber") != "":
		s.putPart(w, r, q.Get("uploadId"), q.Get("partNumber"))
	case r.Method == http.MethodPost && q.Get("uploadId") != "":
		s.complete(w, r, bucket, key, q.Get("uploadId"))
	case r.Method == http.MethodDelete && q.Get("uploadId") != "":
		s.mu.Lock()
		delete(s.uploads, q.Get("uploadId"))
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPut && key != "":
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.objects[bucket][key] = body
		s.mu.Unlock()
		w.Header().Set("ETag", `"put"`)
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodHead && key != "":
		s.stat(w, bucket, key)
	case r.Method == http.MethodGet && key != "":
		s.get(w, r, bucket, key)
	default:
		http.Error(w, "unexpected "+r.Method+" "+r.URL.RequestURI(), http.StatusNotImplemented)
	}
}

func (s *s3fake) initiate(w http.ResponseWriter, bucket, key string) {
	id := fmt.Sprintf("up-%d", len(s.uploads)+1)
	s.mu.Lock()
	s.uploads[id] = &upload{bucket: bucket, key: key, parts: map[int][]byte{}}
	s.mu.Unlock()
	writeXML(w, initResult{Bucket: bucket, Key: key, UploadID: id})
}

func (s *s3fake) putPart(w http.ResponseWriter, r *http.Request, id, part string) {
	body, _ := io.ReadAll(r.Body)
	n := 0
	fmt.Sscanf(part, "%d", &n)
	s.mu.Lock()
	up := s.uploads[id]
	if up != nil {
		up.parts[n] = body
	}
	s.mu.Unlock()
	if up == nil {
		writeS3Error(w, http.StatusNotFound, "NoSuchUpload", "", "")
		return
	}
	w.Header().Set("ETag", fmt.Sprintf(`"part-%d"`, n))
	w.WriteHeader(http.StatusOK)
}

func (s *s3fake) complete(w http.ResponseWriter, r *http.Request, bucket, key, id string) {
	io.Copy(io.Discard, r.Body)
	s.mu.Lock()
	up := s.uploads[id]
	if up == nil {
		s.mu.Unlock()
		writeS3Error(w, http.StatusNotFound, "NoSuchUpload", bucket, key)
		return
	}
	var buf bytes.Buffer
	for i := 1; i <= len(up.parts); i++ {
		buf.Write(up.parts[i])
	}
	s.objects[bucket][key] = buf.Bytes()
	delete(s.uploads, id)
	s.mu.Unlock()
	writeXML(w, completeResult{Bucket: bucket, Key: key, ETag: `"done"`, Location: "https://example/" + key})
}

func (s *s3fake) stat(w http.ResponseWriter, bucket, key string) {
	s.mu.Lock()
	body, ok := s.objects[bucket][key]
	s.mu.Unlock()
	if !ok {
		writeS3Error(w, http.StatusNotFound, "NoSuchKey", bucket, key)
		return
	}
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	w.Header().Set("ETag", `"obj"`)
	w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
	w.WriteHeader(http.StatusOK)
}

func (s *s3fake) get(w http.ResponseWriter, r *http.Request, bucket, key string) {
	s.mu.Lock()
	body, ok := s.objects[bucket][key]
	s.mu.Unlock()
	if !ok {
		writeS3Error(w, http.StatusNotFound, "NoSuchKey", bucket, key)
		return
	}
	start, end := 0, len(body)-1
	if rng := r.Header.Get("Range"); strings.HasPrefix(rng, "bytes=") {
		fmt.Sscanf(strings.TrimPrefix(rng, "bytes="), "%d-%d", &start, &end)
		if end >= len(body) {
			end = len(body) - 1
		}
		if start < 0 || start > end {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("ETag", `"obj"`)
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body[start : end+1])
		return
	}
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
	w.Header().Set("ETag", `"obj"`)
	_, _ = w.Write(body)
}

func splitPath(p string) (bucket, key string) {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return "", ""
	}
	bucket, key, _ = strings.Cut(p, "/")
	if unescaped, err := url.PathUnescape(key); err == nil {
		key = unescaped
	}
	return bucket, key
}

func writeS3Error(w http.ResponseWriter, status int, code, bucket, key string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	fmt.Fprintf(w, "<Error><Code>%s</Code><Message>%s</Message><Bucket>%s</Bucket><Key>%s</Key></Error>", code, code, bucket, key)
}

func writeXML(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/xml")
	enc := xml.NewEncoder(w)
	_ = enc.Encode(v)
}

type initResult struct {
	XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	UploadID string   `xml:"UploadId"`
}

type completeResult struct {
	XMLName  xml.Name `xml:"CompleteMultipartUploadResult"`
	Location string   `xml:"Location"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	ETag     string   `xml:"ETag"`
}
