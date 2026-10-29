package core

import (
	"github.com/minio/minio-go/v7"
)

// readFirst returns the first non-empty object. Later readers cover public
// copies (avatars, wallpapers, stickers) when the canonical prefix misses.
func readFirst(reads ...func() ([]byte, error)) ([]byte, error) {
	var last error
	for _, read := range reads {
		b, err := read()
		if err == nil && len(b) > 0 {
			return b, nil
		}
		if err != nil {
			last = err
		}
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
