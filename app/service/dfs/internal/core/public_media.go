package core

import (
	"errors"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/minio_util"
)

// readFirst returns the first non-empty object. Later readers cover public
// copies (avatars, wallpapers, stickers) when the canonical prefix misses.
// A short read at the end of an object that does exist stops the search:
// the next prefix is a different file, not the rest of this one.
func readFirst(reads ...func() ([]byte, error)) ([]byte, error) {
	var hard, last error
	for _, read := range reads {
		b, err := read()
		if errors.Is(err, io.EOF) {
			if b == nil {
				b = []byte{}
			}
			return b, nil
		}
		if err == nil && len(b) > 0 {
			return b, nil
		}
		if err != nil && !errors.Is(err, minio_util.ErrNotFound) {
			// A later miss must not turn a timeout or a refused read into
			// "object does not exist". Callers map that miss to
			// FILE_ID_INVALID.
			if hard == nil {
				hard = err
			}
			continue
		}
		if errors.Is(err, minio_util.ErrNotFound) && last == nil {
			last = err
		}
	}
	if hard != nil {
		return nil, hard
	}
	if last == nil {
		return []byte{}, nil
	}
	return nil, last
}

// putProfilePhoto writes the canonical photo object (attachments/photos) and
// a public copy (public media/avatars). upload.getFile keeps reading the
// canonical object. The public copy is what TELLOMI_S3_PUBLIC_MEDIA_BASE_URL serves.
func (c *DfsCore) putProfilePhoto(path string, b []byte) (minio.UploadInfo, error) {
	info, err := c.svcCtx.Dao.PutPhotoFile(c.ctx, path, b)
	if err != nil {
		return info, err
	}
	if _, err = c.svcCtx.Dao.PutAvatarFile(c.ctx, path, b); err != nil {
		c.Logger.Errorf("dfs.putAvatarFile (%s) error: %v", path, err)
		return info, err
	}
	return info, nil
}

func (c *DfsCore) putProfileVideo(path string, b []byte) (minio.UploadInfo, error) {
	info, err := c.svcCtx.Dao.PutVideoFile(c.ctx, path, b)
	if err != nil {
		return info, err
	}
	if _, err = c.svcCtx.Dao.PutAvatarFile(c.ctx, path, b); err != nil {
		c.Logger.Errorf("dfs.putAvatarFile (%s) error: %v", path, err)
		return info, err
	}
	return info, nil
}
