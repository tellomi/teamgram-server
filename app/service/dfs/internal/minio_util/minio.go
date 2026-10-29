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
	"fmt"
	"io"
	"net/http"
	"path/filepath"
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
	region := cfg.Region
	if region == "" {
		region = "auto"
	}
	lookup := minio.BucketLookupPath
	if cfg.PathStyleDisabled {
		lookup = minio.BucketLookupDNS
	}
	var transport http.RoundTripper
	if cfg.Transport != nil {
		transport = cfg.Transport
	} else {
		base, err := minio.DefaultTransport(cfg.UseSSL)
		if err != nil {
			return nil, fmt.Errorf("object storage transport: %w", err)
		}
		transport = base
	}
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure:       cfg.UseSSL,
		Region:       region,
		BucketLookup: lookup,
		Transport:    guardTransport{base: transport},
	})
	if err != nil {
		return nil, fmt.Errorf("object storage client: %w", err)
	}
	// A fixed region skips GetBucketLocation. R2 expects "auto".
	u := &MinioUtil{c: &cfg, client: client}
	if err := u.verifyBuckets(context.Background()); err != nil {
		return nil, err
	}
	return u, nil
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

func (m *MinioUtil) put(ctx context.Context, class, path string, r io.Reader, size int64, contentType string) (minio.UploadInfo, error) {
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
	return info, nil
}

func (m *MinioUtil) putBytes(ctx context.Context, class, path string, buf []byte) (minio.UploadInfo, error) {
	return m.put(ctx, class, path, bytes.NewReader(buf), int64(len(buf)), contentTypeFor(path))
}

func (m *MinioUtil) getRange(ctx context.Context, class, path string, offset int64, limit int32) ([]byte, error) {
	bucket, key, err := resolveLocation(m.c, class, path)
	if err != nil {
		return nil, err
	}
	if limit < 0 {
		return nil, fmt.Errorf("invalid read limit %d", limit)
	}
	object, err := m.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		logx.WithContext(ctx).Errorf("GetObject %s/%s error: %v", bucket, key, err)
		return nil, err
	}
	defer object.Close()

	buf := make([]byte, limit)
	n, err := object.ReadAt(buf, offset)
	buf = buf[:n]
	if n > 0 {
		return buf, nil
	}
	if err != nil {
		logx.WithContext(ctx).Errorf("GetObject %s/%s (%s) error: %v", bucket, key, path, err)
	}
	return buf, err
}

func (m *MinioUtil) readAll(ctx context.Context, class, path string) ([]byte, error) {
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

func (m *MinioUtil) resolveRead(bucketOrClass, path string) (string, string, error) {
	switch bucketOrClass {
	case ClassDocuments, ClassPhotos, ClassVideos, ClassEncryptedFiles,
		ClassAvatars, ClassStickers, ClassEmoji, ClassWallpapers, ClassBackups:
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

	buf := make([]byte, limit)
	n, err := object.ReadAt(buf, offset)
	buf = buf[:n]
	if n > 0 {
		return buf, nil
	}
	if err != nil {
		logx.WithContext(ctx).Errorf("GetFile (%s) error: %v", path, err)
	}
	return buf, err
}

func (m *MinioUtil) GetPhotoFile(ctx context.Context, path string, offset int64, limit int32) ([]byte, error) {
	return m.getRange(ctx, ClassPhotos, path, offset, limit)
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
