package crashreport

import (
	"context"
	"time"

	"github.com/teamgram/teamgram-server/app/service/dfs/objectstore"
)

// OpenObjects uses the same TELLOMI_S3_* client as media uploads.
// Unset variables return nil, nil.
func OpenObjects() (Objects, error) {
	c, err := objectstore.Open()
	if err != nil || c == nil {
		return nil, err
	}
	return s3Objects{c: c}, nil
}

type s3Objects struct {
	c *objectstore.Client
}

func (s s3Objects) Copy(ctx context.Context, srcClass, srcPath, dstPath string) error {
	return s.c.CopyToCrashReport(ctx, srcClass, srcPath, dstPath)
}

func (s s3Objects) Put(ctx context.Context, path, contentType string, body []byte) error {
	return s.c.PutCrashReport(ctx, path, contentType, body)
}

func (s s3Objects) Presign(ctx context.Context, path string, ttl time.Duration) (string, error) {
	return s.c.PresignCrashReport(ctx, path, ttl)
}
