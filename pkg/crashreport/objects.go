package crashreport

import (
	"context"
	"time"
)

// Objects copies and writes crash-report files and signs a download URL.
type Objects interface {
	Copy(ctx context.Context, srcClass, srcPath, dstPath string) error
	Put(ctx context.Context, path, contentType string, body []byte) error
	Presign(ctx context.Context, path string, ttl time.Duration) (string, error)
}
