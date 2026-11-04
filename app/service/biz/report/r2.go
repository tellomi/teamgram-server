package report

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/zeromicro/go-zero/core/logx"
)

// r2Store writes evidence to an S3-compatible bucket. Cloudflare R2 uses
// region "auto". The object key is reports/{id}.json.
type r2Store struct {
	client *minio.Client
	bucket string
}

func (s r2Store) Put(ctx context.Context, key string, body []byte, contentType string) error {
	if !strings.HasPrefix(key, "reports/") || strings.Contains(key, "..") {
		return errors.New("report snapshot key must stay under reports/")
	}
	if s.client == nil || s.bucket == "" {
		return errors.New("report snapshot store is not configured")
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(body), int64(len(body)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

// openSnapshot returns a store when TELLOMI_REPORT_SNAPSHOT=1 and the S3
// settings are complete. A missing setting leaves snapshots off and does
// not stop the process.
func openSnapshot() ObjectStore {
	if os.Getenv(EnvSnapshot) != "1" {
		return nil
	}
	endpoint := strings.TrimSpace(os.Getenv("TELLOMI_S3_ENDPOINT"))
	keyID := strings.TrimSpace(os.Getenv("TELLOMI_S3_ACCESS_KEY_ID"))
	secret := os.Getenv("TELLOMI_S3_SECRET_ACCESS_KEY")
	bucket := strings.TrimSpace(os.Getenv(EnvSnapshotBucket))
	if bucket == "" {
		bucket = strings.TrimSpace(os.Getenv("TELLOMI_S3_BUCKET_BACKUPS"))
	}
	if endpoint == "" || keyID == "" || secret == "" || bucket == "" {
		logx.Error("report snapshot is enabled but object storage settings are incomplete")
		return nil
	}
	endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	useSSL := true
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TELLOMI_S3_USE_SSL"))) {
	case "0", "false", "no":
		useSSL = false
	}
	region := strings.TrimSpace(os.Getenv("TELLOMI_S3_REGION"))
	if region == "" {
		region = "auto"
	}
	lookup := minio.BucketLookupPath
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TELLOMI_S3_FORCE_PATH_STYLE")), "false") {
		lookup = minio.BucketLookupDNS
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(keyID, secret, ""),
		Secure:       useSSL,
		Region:       region,
		BucketLookup: lookup,
	})
	if err != nil {
		logx.Error("report snapshot client was not created")
		return nil
	}
	return r2Store{client: client, bucket: bucket}
}
