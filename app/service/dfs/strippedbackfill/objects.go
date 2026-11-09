package strippedbackfill

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/dfs/internal/minio_util"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

// Objects reads a profile JPEG from the photos bucket, then the public
// avatar copy. An empty public object does not hide the canonical JPEG.
type Objects struct {
	Minio *minio_util.MinioUtil
}

func (o Objects) Read(ctx context.Context, path string) ([]byte, error) {
	if o.Minio == nil {
		return nil, errNoStore
	}
	b, err := o.Minio.GetPhotoFileData(ctx, path)
	if err == nil && len(b) > 0 {
		return b, nil
	}
	b2, err2 := o.Minio.GetAvatarFile(ctx, path, 0, 8<<20)
	if err2 == nil && len(b2) > 0 {
		return b2, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, err2
}

var errNoStore = errString("object storage is not configured")

type errString string

func (e errString) Error() string { return string(e) }

// Redis drops the photo, user, and chat cache entries for one photo.
type Redis struct {
	Client *redis.Redis
}

func (r Redis) Drop(ctx context.Context, keys ...string) error {
	if r.Client == nil || len(keys) == 0 {
		return nil
	}
	_, err := r.Client.DelCtx(ctx, keys...)
	return err
}
