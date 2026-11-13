// Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package minio_util

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/teamgram/teamgram-server/app/service/dfs/internal/model"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/zeromicro/go-zero/core/logx"
)

// MinioUtil talks to any S3-compatible endpoint. MinIO is the local default.
// Cloudflare R2 works with region "auto" and path-style addressing. The client
// never calls ListBuckets or MakeBucket.
type MinioUtil struct {
	c      *MinioConfig
	client *minio.Client
	// chunks caches photo and document ranges from upload.getFile.
	// Nil disables it (both caps are 0).
	chunks *chunkCache
	hot    *hotTier
}

// BucketError is returned when a configured bucket is missing or not accessible.
type BucketError struct {
	Bucket  string
	EnvVar  string
	Missing bool
	Err     error
}

func (e *BucketError) Error() string {
	if e.Missing {
		return fmt.Sprintf("object storage bucket %q (%s) does not exist; create it before starting. This service does not call MakeBucket or ListBuckets", e.Bucket, e.EnvVar)
	}
	return fmt.Sprintf("object storage bucket %q (%s) is not accessible: %v. This service does not call MakeBucket or ListBuckets", e.Bucket, e.EnvVar, e.Err)
}

func (e *BucketError) Unwrap() error { return e.Err }

// ErrNotFound is a missing object. Callers that try another prefix should
// not log it; a photo often lives under only one of photos/ and avatars/.
var ErrNotFound = errors.New("object does not exist")

const (
	r2DialTimeout           = 3 * time.Second
	r2TLSHandshakeTimeout   = 3 * time.Second
	r2ResponseHeaderTimeout = 5 * time.Second
	r2KeepAlive             = 30 * time.Second
	r2IdleConnTimeout       = 5 * time.Minute
	r2MaxIdleConns          = 256
	r2MaxIdleConnsPerHost   = 32
	r2WarmupConns           = 4
)

// configureObjectTransport turns on keep-alive and a connection pool for
// the (usually R2) object-storage endpoint. Chengdu to R2 often pays
// 0.8–1.5s for a new TLS session, so idle connections are kept longer
// than Go's default.
func configureObjectTransport(t *http.Transport) *http.Transport {
	if t == nil {
		return t
	}
	t.DisableKeepAlives = false
	t.MaxIdleConns = r2MaxIdleConns
	t.MaxIdleConnsPerHost = r2MaxIdleConnsPerHost
	t.IdleConnTimeout = r2IdleConnTimeout
	t.DialContext = (&net.Dialer{
		Timeout:   r2DialTimeout,
		KeepAlive: r2KeepAlive,
	}).DialContext
	t.TLSHandshakeTimeout = r2TLSHandshakeTimeout
	t.ResponseHeaderTimeout = r2ResponseHeaderTimeout
	return t
}

func MustNewMinioClient(c *MinioConfig) *MinioUtil {
	u, err := NewMinioClient(c)
	if err != nil {
		logx.Must(err)
	}
	return u
}

// NewMinioClient builds an S3 client and checks that every configured bucket
// already exists. It does not create buckets.
func NewMinioClient(c *MinioConfig) (*MinioUtil, error) {
	if c == nil {
		return nil, fmt.Errorf("object storage config is nil")
	}
	cfg := *c
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	if cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, fmt.Errorf("object storage credentials are empty (set %s and %s)", EnvAccessKeyID, EnvSecretAccessKey)
	}
	applyHotDefaults(&cfg.hot)
	if err := validateHotTier(cfg.hot, cfg.HTTPProxy); err != nil {
		return nil, err
	}
	region := cfg.Region
	if region == "" {
		region = "auto"
	}
	lookup := minio.BucketLookupPath
	if cfg.pathStyleDisabled {
		lookup = minio.BucketLookupDNS
	}
	var transport http.RoundTripper
	if cfg.transport != nil {
		if base, ok := cfg.transport.(*http.Transport); ok {
			// Keep the caller's TLS config (tests pin InsecureSkipVerify)
			// and any timeouts they already set. Still apply keep-alive
			// and pool caps.
			dial := base.DialContext
			tlsCfg := base.TLSClientConfig
			tlsTO := base.TLSHandshakeTimeout
			hdrTO := base.ResponseHeaderTimeout
			configureObjectTransport(base)
			if dial != nil {
				base.DialContext = dial
			}
			if tlsCfg != nil {
				base.TLSClientConfig = tlsCfg
			}
			if tlsTO > 0 {
				base.TLSHandshakeTimeout = tlsTO
			}
			if hdrTO > 0 {
				base.ResponseHeaderTimeout = hdrTO
			}
		}
		transport = cfg.transport
	} else {
		base, err := minio.DefaultTransport(cfg.UseSSL)
		if err != nil {
			return nil, fmt.Errorf("object storage transport: %w", err)
		}
		// Default dial is 30s and the header wait is a minute. upload.getFile's
		// RPC deadline is shorter, so a dead endpoint becomes
		// "context deadline exceeded" before the sticker fallback runs.
		// Keep-alives and pool caps, plus the same dial/TLS/header
		// timeouts as master so a hung TELLOMI_S3_HTTP_PROXY does not
		// fall back to a direct dial.
		transport = configureObjectTransport(base)
	}
	var err error
	transport, err = applyObjectHTTPProxy(transport, cfg.HTTPProxy, cfg.Endpoint, cfg.UseSSL)
	if err != nil {
		return nil, err
	}
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure:       cfg.UseSSL,
		Region:       region,
		BucketLookup: lookup,
		// guard rejects bucket admin calls. fastFirstByte retries a GET/HEAD
		// whose first byte is late, and stops well under five seconds.
		// Puts keep the transport header timeout and are not wrapped.
		Transport: fastFirstByte{base: guardTransport{base: transport}},
	})
	if err != nil {
		return nil, fmt.Errorf("object storage client: %w", err)
	}
	// A fixed region skips GetBucketLocation. R2 expects "auto".
	u := &MinioUtil{c: &cfg, client: client, chunks: newChunkCacheFromEnv()}
	if err := u.verifyBuckets(context.Background()); err != nil {
		return nil, err
	}
	if cfg.hot.Enabled {
		ht, err := newHotTier(cfg.hot, u)
		if err != nil {
			return nil, err
		}
		u.hot = ht
		if !cfg.hot.noLoop {
			u.hot.start()
		}
	}
	u.warmup(context.Background())
	return u, nil
}

// warmup opens a few HTTP connections to the object store and leaves them
// in the idle pool so the first upload.getFile after a deploy does not
// pay a cold TCP+TLS handshake to R2. A failed warmup does not prevent
// startup; the next request will dial again.
func (m *MinioUtil) warmup(ctx context.Context) {
	if m == nil || m.client == nil {
		return
	}
	names, err := bucketsToCheck(m.c)
	if err != nil || len(names) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < r2WarmupConns; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = m.client.BucketExists(ctx, names[0])
		}()
	}
	wg.Wait()
}

func (m *MinioUtil) verifyBuckets(ctx context.Context) error {
	names, err := bucketsToCheck(m.c)
	if err != nil {
		return err
	}
	attempts := m.c.BucketProbeAttempts
	if attempts <= 0 {
		attempts = 12
	}
	for _, name := range names {
		if err := m.probeBucket(ctx, name, attempts); err != nil {
			return err
		}
	}
	return nil
}

func (m *MinioUtil) probeBucket(ctx context.Context, bucket string, attempts int) error {
	var last error
	for i := 0; i < attempts; i++ {
		exists, err := m.client.BucketExists(ctx, bucket)
		if err == nil && exists {
			return nil
		}
		envVar := envVarForBucket(m.c, bucket)
		if err == nil {
			last = &BucketError{Bucket: bucket, EnvVar: envVar, Missing: true}
		} else {
			last = &BucketError{Bucket: bucket, EnvVar: envVar, Err: err}
		}
		if i+1 < attempts {
			time.Sleep(time.Second)
		}
	}
	return last
}

func contentTypeFor(path string) string {
	if ext := filepath.Ext(path); model.IsFileExtImage(ext) {
		return model.GetImageMimeType(ext)
	}
	return "binary/octet-stream"
}

func (m *MinioUtil) hotEnabled(class string) bool {
	return m != nil && m.hot != nil && hotChatClass(class)
}

// StoresHot reports whether new objects of class are written to the local
// hot tier instead of waiting on object storage.
func (m *MinioUtil) StoresHot(class string) bool {
	return m.hotEnabled(class)
}

// EnsureLocal writes body at the key Get*File reads. A hot-tier object that
// is already on disk is left in place, so a reused photo does not upload
// those bytes to R2 again. With the hot tier off this is the normal put.
func (m *MinioUtil) EnsureLocal(ctx context.Context, class, path string, body []byte) error {
	if m == nil {
		return fmt.Errorf("object storage client is nil")
	}
	if path == "" {
		return fmt.Errorf("object key is empty")
	}
	if m.hotEnabled(class) && m.hot.has(class, path) {
		return nil
	}
	_, err := m.putBytes(ctx, class, path, body)
	return err
}

// ReadHot returns one range from the local hot tier. ok is false when that
// key is not a local file; the caller then tries another key or R2.
func (m *MinioUtil) ReadHot(class, path string, offset int64, limit int32) ([]byte, bool) {
	if !m.hotEnabled(class) || path == "" || !m.hot.has(class, path) {
		return nil, false
	}
	b, err := m.hot.readRange(class, path, offset, limit)
	if err != nil {
		return nil, false
	}
	return b, true
}

func (m *MinioUtil) putRemote(ctx context.Context, class, path string, r io.Reader, size int64, contentType string) (minio.UploadInfo, error) {
	bucket, key, err := resolveLocation(m.c, class, path)
	if err != nil {
		return minio.UploadInfo{}, err
	}
	if contentType == "" {
		contentType = contentTypeFor(path)
	}
	info, err := m.client.PutObject(ctx, bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	return info, err
}

func (m *MinioUtil) put(ctx context.Context, class, path string, r io.Reader, size int64, contentType string) (minio.UploadInfo, error) {
	if m.hotEnabled(class) {
		meta, err := m.hot.write(class, path, r, size)
		if err != nil {
			return minio.UploadInfo{}, err
		}
		if m.chunks != nil {
			m.chunks.forgetPath(class, path)
		}
		bucket, key, err := resolveLocation(m.c, class, path)
		if err != nil {
			return minio.UploadInfo{}, err
		}
		return minio.UploadInfo{Bucket: bucket, Key: key, Size: meta.Size, ETag: `"` + meta.SHA256 + `"`}, nil
	}
	bucket, key, err := resolveLocation(m.c, class, path)
	if err != nil {
		return minio.UploadInfo{}, err
	}
	if contentType == "" {
		contentType = contentTypeFor(path)
	}
	info, err := m.client.PutObject(ctx, bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		logx.WithContext(ctx).Errorf("PutObject %s/%s (%s) error: %v", bucket, key, class, err)
		return info, err
	}
	// A miss recorded before this write must not hide the new object.
	if m.chunks != nil {
		m.chunks.forgetPath(class, path)
	}
	return info, nil
}

// RemoveClassObject deletes the object Get*File reads for a chat-media class.
// Only private chat classes are accepted: public media (avatars, stickers,
// emoji, wallpapers) is never removed through this path. A missing object
// is not an error.
func (m *MinioUtil) RemoveClassObject(ctx context.Context, class, path string) error {
	switch class {
	case ClassDocuments, ClassPhotos, ClassVideos, ClassEncryptedFiles:
	default:
		// crash-reports is not a chat-media class, so this path refuses it.
		return fmt.Errorf("refusing to remove %s/%s: only chat media classes can be removed", class, path)
	}
	return m.removeKey(ctx, class, path, false)
}

// trashClasses are the only classes account deletion may move. Stickers,
// emoji and wallpapers stay. Avatars are included because a deleted profile
// photo has no other owner; RemoveClassObject still refuses them.
func trashClass(class string) bool {
	switch class {
	case ClassDocuments, ClassPhotos, ClassVideos, ClassEncryptedFiles, ClassAvatars:
		return true
	default:
		return false
	}
}

// Endpoint is the configured S3 host, without credentials.
// CopyClassObject copies one object onto the crash-reports class. The
// destination class is the only one this accepts, so chat media cannot be
// overwritten through it.
func (m *MinioUtil) CopyClassObject(ctx context.Context, srcClass, srcPath, dstClass, dstPath string) error {
	if m == nil || m.client == nil {
		return fmt.Errorf("object storage client is nil")
	}
	if dstClass != ClassCrashReports {
		return fmt.Errorf("refusing to copy into %s", dstClass)
	}
	switch srcClass {
	case ClassDocuments, ClassPhotos, ClassVideos, ClassEncryptedFiles, ClassCrashReports:
	default:
		return fmt.Errorf("refusing to copy from %s", srcClass)
	}
	if m.hotEnabled(srcClass) && m.hot.has(srcClass, srcPath) {
		data, err := m.hot.readRange(srcClass, srcPath, 0, 1<<30)
		if err != nil {
			return err
		}
		_, err = m.putRemote(ctx, dstClass, dstPath, bytes.NewReader(data), int64(len(data)), contentTypeFor(dstPath))
		return err
	}
	srcBucket, srcKey, err := resolveLocation(m.c, srcClass, srcPath)
	if err != nil {
		return err
	}
	dstBucket, dstKey, err := resolveLocation(m.c, dstClass, dstPath)
	if err != nil {
		return err
	}
	_, err = m.client.CopyObject(ctx, minio.CopyDestOptions{Bucket: dstBucket, Object: dstKey}, minio.CopySrcOptions{Bucket: srcBucket, Object: srcKey})
	if err != nil {
		logx.WithContext(ctx).Errorf("CopyObject %s/%s -> %s/%s error: %v", srcBucket, srcKey, dstBucket, dstKey, err)
	}
	return err
}

// PresignClass returns a GET URL for a crash-report object. Other classes
// are refused so this helper cannot sign chat media.
func (m *MinioUtil) PresignClass(ctx context.Context, class, path string, expiry time.Duration) (string, error) {
	if m == nil || m.client == nil {
		return "", fmt.Errorf("object storage client is nil")
	}
	if class != ClassCrashReports {
		return "", fmt.Errorf("refusing to sign %s", class)
	}
	if expiry <= 0 {
		expiry = 15 * time.Minute
	}
	bucket, key, err := resolveLocation(m.c, class, path)
	if err != nil {
		return "", err
	}
	u, err := m.client.PresignedGetObject(ctx, bucket, key, expiry, nil)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (m *MinioUtil) Endpoint() string {
	if m == nil || m.c == nil {
		return ""
	}
	return m.c.Endpoint
}

// MoveToTrash copies the object to trash/<original key> in the same bucket,
// then deletes the original. A missing source is success so a retry after a
// crash is safe. The trash copy is what DeleteTrash removes after the
// retention window.
func (m *MinioUtil) MoveToTrash(ctx context.Context, class, path string) error {
	if m == nil || m.client == nil {
		return fmt.Errorf("object storage client is nil")
	}
	if !trashClass(class) {
		return fmt.Errorf("refusing to trash %s/%s", class, path)
	}
	if m.hotEnabled(class) {
		if err := m.hot.moveToTrash(class, path); err != nil {
			return err
		}
	}
	bucket, key, err := resolveLocation(m.c, class, path)
	if err != nil {
		return err
	}
	trashKey := "trash/" + key
	_, err = m.client.CopyObject(ctx, minio.CopyDestOptions{Bucket: bucket, Object: trashKey}, minio.CopySrcOptions{Bucket: bucket, Object: key})
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		logx.WithContext(ctx).Errorf("CopyObject %s/%s -> %s error: %v", bucket, key, trashKey, err)
		return err
	}
	if err = m.client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{}); err != nil && !isNotFound(err) {
		logx.WithContext(ctx).Errorf("RemoveObject %s/%s error: %v", bucket, key, err)
		return err
	}
	return nil
}

// DeleteTrash removes the trash/ copy of a class object. It does not touch
// the live key.
func (m *MinioUtil) DeleteTrash(ctx context.Context, class, path string) error {
	if !trashClass(class) {
		return fmt.Errorf("refusing to delete trash for %s/%s", class, path)
	}
	return m.removeKey(ctx, class, path, true)
}

func (m *MinioUtil) removeKey(ctx context.Context, class, path string, trash bool) error {
	if m == nil || m.client == nil {
		return fmt.Errorf("object storage client is nil")
	}
	if m.hotEnabled(class) {
		var err error
		if trash {
			err = m.hot.deleteTrash(class, path)
		} else {
			err = m.hot.remove(class, path)
		}
		if err != nil {
			return err
		}
	}
	bucket, key, err := resolveLocation(m.c, class, path)
	if err != nil {
		return err
	}
	if trash {
		key = "trash/" + key
	}
	if err = m.client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{}); err != nil && !isNotFound(err) {
		logx.WithContext(ctx).Errorf("RemoveObject %s/%s error: %v", bucket, key, err)
		return err
	}
	return nil
}

func (m *MinioUtil) putBytes(ctx context.Context, class, path string, buf []byte) (minio.UploadInfo, error) {
	return m.put(ctx, class, path, bytes.NewReader(buf), int64(len(buf)), contentTypeFor(path))
}

// PutClassBytes writes buf at the same bucket and key Get*File would read.
// contentType may be empty; the client then uses the path extension.
func (m *MinioUtil) PutClassBytes(ctx context.Context, class, path, contentType string, buf []byte) (minio.UploadInfo, error) {
	return m.put(ctx, class, path, bytes.NewReader(buf), int64(len(buf)), contentType)
}

// getRange may answer from the chunk LRU. The key is the object identity
// (class, path, offset, limit), not the caller's access_hash. upload.getFile
// and upload.getFileHashes authorize that hash before they call into dfs, on
// every request. A failed check never reaches this cache.
func (m *MinioUtil) getRange(ctx context.Context, class, path string, offset int64, limit int32) ([]byte, error) {
	if m == nil || m.chunks == nil || !cacheableReadClass(class) {
		return m.getRangeUncached(ctx, class, path, offset, limit)
	}
	return m.chunks.getOrLoad(ctx, chunkKey(class, path, offset, limit), func(ctx context.Context) ([]byte, error) {
		return m.getRangeUncached(ctx, class, path, offset, limit)
	})
}

func (m *MinioUtil) getRangeUncached(ctx context.Context, class, path string, offset int64, limit int32) ([]byte, error) {
	if m.hotEnabled(class) && m.hot.has(class, path) {
		return m.hot.readRange(class, path, offset, limit)
	}
	return m.getRangeUncachedRemote(ctx, class, path, offset, limit)
}

func (m *MinioUtil) getRangeUncachedRemote(ctx context.Context, class, path string, offset int64, limit int32) ([]byte, error) {
	bucket, key, err := resolveLocation(m.c, class, path)
	if err != nil {
		return nil, err
	}
	object, err := m.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		logx.WithContext(ctx).Errorf("GetObject %s/%s error: %v", bucket, key, err)
		return nil, err
	}
	defer object.Close()
	// R2 answers 416 when the requested range runs past the object. Clients
	// ask for a fixed chunk (often larger than an avatar). readClamped
	// shortens the range to the bytes that exist.
	return readClamped(ctx, object, offset, limit, bucket+"/"+key)
}

func isNotFound(err error) bool {
	if err == nil || errors.Is(err, ErrNotFound) {
		return err != nil
	}
	resp := minio.ToErrorResponse(err)
	if resp.Code == "NoSuchKey" || resp.StatusCode == http.StatusNotFound {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "key does not exist") || strings.Contains(msg, "nosuchkey")
}

func isUnsatisfiable(err error) bool {
	if err == nil {
		return false
	}
	resp := minio.ToErrorResponse(err)
	if resp.Code == "InvalidRange" || resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "range not satisfiable")
}

func (m *MinioUtil) readAll(ctx context.Context, class, path string) ([]byte, error) {
	if m.hotEnabled(class) && m.hot.has(class, path) {
		return m.hot.readRange(class, path, 0, 1<<30)
	}
	bucket, key, err := resolveLocation(m.c, class, path)
	if err != nil {
		return nil, err
	}
	object, err := m.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		logx.WithContext(ctx).Errorf("GetObject %s/%s error: %v", bucket, key, err)
		return nil, err
	}
	defer object.Close()

	stat, err := object.Stat()
	if err != nil {
		logx.WithContext(ctx).Errorf("StatObject %s/%s error: %v", bucket, key, err)
		return nil, err
	}
	data := make([]byte, stat.Size)
	n, err := object.Read(data)
	data = data[:n]
	if n > 0 {
		return data, nil
	}
	if err != nil {
		logx.WithContext(ctx).Errorf("ReadObject %s/%s error: %v", bucket, key, err)
	}
	return nil, err
}

// GetFileObject reads an object. bucket is a logical class (documents, photos,
// videos, encryptedfiles, avatars, stickers, emoji, wallpapers) or, in legacy
// mode only, a physical bucket name equal to one of those classes.
func (m *MinioUtil) GetFileObject(ctx context.Context, bucket, path string) (*minio.Object, error) {
	physical, key, err := m.resolveRead(bucket, path)
	if err != nil {
		logx.WithContext(ctx).Errorf("GetFileObject error: %v", err)
		return nil, err
	}
	object, err := m.client.GetObject(ctx, physical, key, minio.GetObjectOptions{})
	if err != nil {
		logx.WithContext(ctx).Errorf("GetFileObject error: %v", err)
		return nil, err
	}
	return object, nil
}

// OpenClass opens a chat-media object as a stream. A hot-tier hit is a
// local file; otherwise this is the object storage GET (which uses the
// configured CONNECT proxy when one is set).
func (m *MinioUtil) OpenClass(ctx context.Context, class, path string) (io.ReadCloser, error) {
	if m.hotEnabled(class) && m.hot.has(class, path) {
		f, _, err := m.hot.open(class, path)
		return f, err
	}
	bucket, key, err := resolveLocation(m.c, class, path)
	if err != nil {
		return nil, err
	}
	object, err := m.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if _, err := object.Stat(); err != nil {
		object.Close()
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return object, nil
}

func (m *MinioUtil) resolveRead(bucketOrClass, path string) (string, string, error) {
	switch bucketOrClass {
	case ClassDocuments, ClassPhotos, ClassVideos, ClassEncryptedFiles,
		ClassAvatars, ClassStickers, ClassEmoji, ClassWallpapers, ClassBackups, ClassCrashReports:
		return resolveLocation(m.c, bucketOrClass, path)
	default:
		// Historical callers passed the physical bucket. Map it back to a class
		// when it matches the configured legacy names; otherwise refuse rather
		// than invent a bucket.
		switch bucketOrClass {
		case m.c.Bucket.Documents:
			return resolveLocation(m.c, ClassDocuments, path)
		case m.c.Bucket.Photos:
			return resolveLocation(m.c, ClassPhotos, path)
		case m.c.Bucket.Videos:
			return resolveLocation(m.c, ClassVideos, path)
		case m.c.Bucket.EncryptedFiles:
			return resolveLocation(m.c, ClassEncryptedFiles, path)
		case m.c.Bucket.Attachments:
			return "", "", fmt.Errorf("bucket %q is a physical bucket; pass a logical class and object key", bucketOrClass)
		}
		return "", "", fmt.Errorf("unknown bucket or storage class %q", bucketOrClass)
	}
}

func (m *MinioUtil) GetPhotoFileData(ctx context.Context, path string) ([]byte, error) {
	return m.readAll(ctx, ClassPhotos, path)
}

func (m *MinioUtil) GetFile(ctx context.Context, bucket, path string, offset int64, limit int32) ([]byte, error) {
	if m.hotEnabled(bucket) && m.hot.has(bucket, path) {
		return m.hot.readRange(bucket, path, offset, limit)
	}
	physical, key, err := m.resolveRead(bucket, path)
	if err != nil {
		logx.WithContext(ctx).Errorf("GetFile error: %v", err)
		return nil, err
	}
	object, err := m.client.GetObject(ctx, physical, key, minio.GetObjectOptions{})
	if err != nil {
		logx.WithContext(ctx).Errorf("GetFile error: %v", err)
		return nil, err
	}
	defer object.Close()

	return readClamped(ctx, object, offset, limit, path)
}

func readClamped(ctx context.Context, object *minio.Object, offset int64, limit int32, label string) ([]byte, error) {
	if limit < 0 {
		return nil, fmt.Errorf("invalid read limit %d", limit)
	}
	if limit == 0 {
		return []byte{}, nil
	}
	stat, err := object.Stat()
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		logx.WithContext(ctx).Errorf("StatObject (%s) error: %v", label, err)
		return nil, err
	}
	if offset >= stat.Size {
		return []byte{}, io.EOF
	}
	remain := stat.Size - offset
	if int64(limit) < remain {
		remain = int64(limit)
	}
	buf := make([]byte, remain)
	n, err := object.ReadAt(buf, offset)
	buf = buf[:n]
	if n > 0 {
		return buf, nil
	}
	if isNotFound(err) {
		return nil, ErrNotFound
	}
	if isUnsatisfiable(err) {
		return []byte{}, io.EOF
	}
	if err != nil {
		logx.WithContext(ctx).Errorf("GetObject (%s) error: %v", label, err)
	}
	return buf, err
}

func (m *MinioUtil) GetPhotoFile(ctx context.Context, path string, offset int64, limit int32) ([]byte, error) {
	return m.getRange(ctx, ClassPhotos, path, offset, limit)
}

// Stat returns the object size. A missing object is ErrNotFound.
func (m *MinioUtil) Stat(ctx context.Context, class, path string) (int64, error) {
	if m == nil || m.client == nil {
		return 0, fmt.Errorf("object storage client is nil")
	}
	if m.hotEnabled(class) && m.hot.has(class, path) {
		f, meta, err := m.hot.open(class, path)
		if err != nil {
			return 0, err
		}
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
		return meta.Size, nil
	}
	return m.statRemote(ctx, class, path)
}

func (m *MinioUtil) statRemote(ctx context.Context, class, path string) (int64, error) {
	bucket, key, err := resolveLocation(m.c, class, path)
	if err != nil {
		return 0, err
	}
	info, err := m.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if isNotFound(err) {
			return 0, ErrNotFound
		}
		logx.WithContext(ctx).Errorf("StatObject %s/%s error: %v", bucket, key, err)
		return 0, err
	}
	return info.Size, nil
}

func (m *MinioUtil) PutPhotoFile(ctx context.Context, path string, buf []byte) (minio.UploadInfo, error) {
	return m.putBytes(ctx, ClassPhotos, path, buf)
}

func (m *MinioUtil) PutPhotoFileV2(ctx context.Context, path string, r io.Reader) (minio.UploadInfo, error) {
	return m.put(ctx, ClassPhotos, path, r, -1, contentTypeFor(path))
}

func (m *MinioUtil) GetVideoFile(ctx context.Context, path string, offset int64, limit int32) ([]byte, error) {
	return m.getRange(ctx, ClassVideos, path, offset, limit)
}

func (m *MinioUtil) PutVideoFile(ctx context.Context, path string, buf []byte) (minio.UploadInfo, error) {
	return m.putBytes(ctx, ClassVideos, path, buf)
}

func (m *MinioUtil) PutVideoFileV2(ctx context.Context, path string, r io.Reader) (minio.UploadInfo, error) {
	return m.put(ctx, ClassVideos, path, r, -1, contentTypeFor(path))
}

func (m *MinioUtil) GetDocumentFile(ctx context.Context, path string, offset int64, limit int32) ([]byte, error) {
	return m.getRange(ctx, ClassDocuments, path, offset, limit)
}

func (m *MinioUtil) PutDocumentFile(ctx context.Context, path string, r io.Reader) (minio.UploadInfo, error) {
	return m.put(ctx, ClassDocuments, path, r, -1, contentTypeFor(path))
}

func (m *MinioUtil) FPutDocumentFile(ctx context.Context, path string, filePath string) (minio.UploadInfo, error) {
	if m.hotEnabled(ClassDocuments) {
		f, err := os.Open(filePath)
		if err != nil {
			return minio.UploadInfo{}, err
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			return minio.UploadInfo{}, err
		}
		return m.put(ctx, ClassDocuments, path, f, st.Size(), contentTypeFor(path))
	}
	bucket, key, err := resolveLocation(m.c, ClassDocuments, path)
	if err != nil {
		return minio.UploadInfo{}, err
	}
	info, err := m.client.FPutObject(ctx, bucket, key, filePath, minio.PutObjectOptions{
		ContentType: contentTypeFor(path),
	})
	if err != nil {
		logx.WithContext(ctx).Errorf("FPutObject %s/%s error: %v", bucket, key, err)
	}
	return info, err
}

func (m *MinioUtil) GetEncryptedFile(ctx context.Context, path string, offset int64, limit int32) ([]byte, error) {
	return m.getRange(ctx, ClassEncryptedFiles, path, offset, limit)
}

func (m *MinioUtil) PutEncryptedFile(ctx context.Context, path string, r io.Reader) (minio.UploadInfo, error) {
	// Secret-chat bytes are already client-encrypted. Do not apply MinIO SSE.
	return m.put(ctx, ClassEncryptedFiles, path, r, -1, "binary/octet-stream")
}

// PutAvatarFile stores a profile photo or profile video in the public bucket.
func (m *MinioUtil) PutAvatarFile(ctx context.Context, path string, buf []byte) (minio.UploadInfo, error) {
	return m.putBytes(ctx, ClassAvatars, path, buf)
}

func (m *MinioUtil) GetAvatarFile(ctx context.Context, path string, offset int64, limit int32) ([]byte, error) {
	return m.getRange(ctx, ClassAvatars, path, offset, limit)
}

func (m *MinioUtil) PutStickerFile(ctx context.Context, path string, r io.Reader) (minio.UploadInfo, error) {
	return m.put(ctx, ClassStickers, path, r, -1, contentTypeFor(path))
}

func (m *MinioUtil) PutStickerBytes(ctx context.Context, path string, buf []byte) (minio.UploadInfo, error) {
	return m.putBytes(ctx, ClassStickers, path, buf)
}

func (m *MinioUtil) GetStickerFile(ctx context.Context, path string, offset int64, limit int32) ([]byte, error) {
	return m.getRange(ctx, ClassStickers, path, offset, limit)
}

// ClassHead is the size and ETag of one object under a storage class.
type ClassHead struct {
	Size int64
	ETag string
}

// ListClass returns every object under a class prefix. The map key is the
// path inside the class (stickers/42.dat is "42.dat"). A missing bucket
// returns the client error so the caller can fall back to uploading.
func (m *MinioUtil) ListClass(ctx context.Context, class string) (map[string]ClassHead, error) {
	if m == nil || m.client == nil {
		return nil, fmt.Errorf("object storage client is nil")
	}
	bucket, key, err := resolveLocation(m.c, class, "_")
	if err != nil {
		return nil, err
	}
	prefix := strings.TrimSuffix(key, "_")
	out := map[string]ClassHead{}
	for info := range m.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if info.Err != nil {
			return nil, info.Err
		}
		rel := strings.TrimPrefix(info.Key, prefix)
		if rel == "" {
			continue
		}
		out[rel] = ClassHead{Size: info.Size, ETag: info.ETag}
	}
	return out, nil
}

// StatClass returns the stored size of a class object. A missing object is ErrNotFound.
func (m *MinioUtil) StatClass(ctx context.Context, class, path string) (int64, error) {
	return m.Stat(ctx, class, path)
}

func (m *MinioUtil) PutEmojiFile(ctx context.Context, path string, r io.Reader) (minio.UploadInfo, error) {
	return m.put(ctx, ClassEmoji, path, r, -1, contentTypeFor(path))
}

func (m *MinioUtil) PutEmojiBytes(ctx context.Context, path string, buf []byte) (minio.UploadInfo, error) {
	return m.putBytes(ctx, ClassEmoji, path, buf)
}

func (m *MinioUtil) GetEmojiFile(ctx context.Context, path string, offset int64, limit int32) ([]byte, error) {
	return m.getRange(ctx, ClassEmoji, path, offset, limit)
}

func (m *MinioUtil) PutWallpaperFile(ctx context.Context, path string, r io.Reader) (minio.UploadInfo, error) {
	return m.put(ctx, ClassWallpapers, path, r, -1, contentTypeFor(path))
}

func (m *MinioUtil) PutWallpaperBytes(ctx context.Context, path string, buf []byte) (minio.UploadInfo, error) {
	return m.putBytes(ctx, ClassWallpapers, path, buf)
}

func (m *MinioUtil) GetWallpaperFile(ctx context.Context, path string, offset int64, limit int32) ([]byte, error) {
	return m.getRange(ctx, ClassWallpapers, path, offset, limit)
}

func (m *MinioUtil) PutBackup(ctx context.Context, path string, r io.Reader, size int64) (minio.UploadInfo, error) {
	return m.put(ctx, ClassBackups, path, r, size, "application/octet-stream")
}

func (m *MinioUtil) GetBackup(ctx context.Context, path string, offset int64, limit int32) ([]byte, error) {
	return m.getRange(ctx, ClassBackups, path, offset, limit)
}

// PublicMediaURL returns a CDN URL for a public object when
// TELLOMI_S3_PUBLIC_MEDIA_BASE_URL is set.
func (m *MinioUtil) PublicMediaURL(class, path string) (string, bool) {
	if m.c.PublicMediaBaseURL == "" {
		return "", false
	}
	switch class {
	case ClassAvatars, ClassStickers, ClassEmoji, ClassWallpapers:
	default:
		return "", false
	}
	_, key, err := resolveLocation(m.c, class, path)
	if err != nil {
		return "", false
	}
	return joinPublicURL(m.c.PublicMediaBaseURL, key), true
}

// AttachmentsURL returns a CDN URL for a private object when
// TELLOMI_S3_ATTACHMENTS_BASE_URL is set. Callers must still authorize the download.
func (m *MinioUtil) AttachmentsURL(class, path string) (string, bool) {
	if m.c.AttachmentsBaseURL == "" {
		return "", false
	}
	switch class {
	case ClassDocuments, ClassPhotos, ClassVideos, ClassEncryptedFiles:
	default:
		return "", false
	}
	_, key, err := resolveLocation(m.c, class, path)
	if err != nil {
		return "", false
	}
	return joinPublicURL(m.c.AttachmentsBaseURL, key), true
}
