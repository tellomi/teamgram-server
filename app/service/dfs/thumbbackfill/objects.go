package thumbbackfill

import (
	"context"
	"errors"
	"io"

	"github.com/teamgram/teamgram-server/app/service/dfs/internal/minio_util"
)

// maxSource is the largest document the command reads into memory to rebuild
// one thumb. A bigger file is flagged rather than loaded.
const maxSource int32 = 64 << 20

// Objects is the dfs S3 client.
type Objects struct {
	Minio *minio_util.MinioUtil
}

func (o Objects) Exists(ctx context.Context, path string) (bool, error) {
	if o.Minio == nil {
		return false, errors.New("object storage is nil")
	}
	n, err := o.Minio.Stat(ctx, minio_util.ClassPhotos, path)
	if errors.Is(err, minio_util.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (o Objects) ReadDocument(ctx context.Context, path string) ([]byte, error) {
	if o.Minio == nil {
		return nil, errors.New("object storage is nil")
	}
	buf, err := o.Minio.GetDocumentFile(ctx, path, 0, maxSource+1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(buf) > int(maxSource) {
		return nil, errors.New("document is larger than 64MiB")
	}
	return buf, nil
}

func (o Objects) PutPhoto(ctx context.Context, path string, body []byte) error {
	if o.Minio == nil {
		return errors.New("object storage is nil")
	}
	_, err := o.Minio.PutPhotoFile(ctx, path, body)
	return err
}
