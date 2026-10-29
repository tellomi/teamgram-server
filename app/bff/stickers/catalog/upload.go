package catalog

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// uploadAssets writes seeded documents into the public media bucket.
// Keys match docs/storage.md: stickers/<id>.dat and emoji/<id>.dat.
// Missing S3 settings skip the upload so the catalog can still be served.
func uploadAssets(ctx context.Context, packs []*Pack) error {
	endpoint := strings.TrimSpace(os.Getenv("TELLOMI_S3_ENDPOINT"))
	keyID := os.Getenv("TELLOMI_S3_ACCESS_KEY_ID")
	secret := os.Getenv("TELLOMI_S3_SECRET_ACCESS_KEY")
	bucket := strings.TrimSpace(os.Getenv("TELLOMI_S3_BUCKET_PUBLIC_MEDIA"))
	if endpoint == "" || keyID == "" || secret == "" || bucket == "" {
		return nil
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("sticker storage endpoint: %w", err)
	}
	host := u.Host
	secure := u.Scheme == "https"
	if host == "" {
		host = endpoint
		secure = strings.HasPrefix(endpoint, "https://")
	}
	region := strings.TrimSpace(os.Getenv("TELLOMI_S3_REGION"))
	if region == "" {
		region = "auto"
	}
	client, err := minio.New(host, &minio.Options{
		Creds:        credentials.NewStaticV4(keyID, secret, ""),
		Secure:       secure,
		Region:       region,
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, p := range packs {
		for _, item := range p.Items {
			objectKey := item.StorageClass + "/" + item.ObjectPath
			if seen[objectKey] {
				continue
			}
			seen[objectKey] = true
			body, err := assetFS.ReadFile(item.AssetPath)
			if err != nil {
				return fmt.Errorf("read %s: %w", item.AssetPath, err)
			}
			contentType := item.MIME
			_, err = client.PutObject(ctx, bucket, objectKey, bytes.NewReader(body), int64(len(body)), minio.PutObjectOptions{
				ContentType: contentType,
			})
			if err != nil {
				return fmt.Errorf("put %s/%s: %w", bucket, objectKey, err)
			}
		}
	}
	return nil
}
