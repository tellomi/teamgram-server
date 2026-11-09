package core

import (
	"errors"
	"fmt"
	"io"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/minio_util"
)

// peerPhotoSizeOrder is the profile ladder clients ask for.
// big is the chat header and profile (c, 640). Otherwise the dialog
// list and message avatars (a, 160). Later sizes are fallbacks when
// that object was never written (a small source has no c).
func peerPhotoSizeOrder(big bool) []string {
	if big {
		return []string{
			mtproto.PhotoSZCType,
			mtproto.PhotoSZDType,
			mtproto.PhotoSZBType,
			mtproto.PhotoSZAType,
		}
	}
	return []string{
		mtproto.PhotoSZAType,
		mtproto.PhotoSZBType,
		mtproto.PhotoSZCType,
		mtproto.PhotoSZDType,
	}
}

// readPeerPhoto loads the JPEG for inputPeerPhotoFileLocation.
// An empty success is not a missing file: official clients cache it
// and keep the placeholder avatar.
func (c *DfsCore) readPeerPhoto(photoID int64, big bool, offset int64, limit int32) ([]byte, error) {
	if photoID == 0 || c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil, mtproto.ErrLocationInvalid
	}
	var last error
	for _, sz := range peerPhotoSizeOrder(big) {
		path := fmt.Sprintf("%s/%d.dat", sz, photoID)
		if p := c.svcCtx.Dao.PhotoObjectPath(c.ctx, photoID, sz); p != "" {
			path = p
		}
		b, err := c.readAvatarThenPhoto(path, offset, limit)
		if len(b) > 0 {
			return b, nil
		}
		if errors.Is(err, minio_util.ErrNotFound) {
			last = err
			continue
		}
		// offset 0 and an empty object: try the next size. A later
		// offset is the end of the object we already started, not a
		// cue to swap in a different file.
		if err == nil || errors.Is(err, io.EOF) {
			if offset > 0 {
				return []byte{}, nil
			}
			last = err
			continue
		}
		return nil, err
	}
	if last == nil {
		last = mtproto.ErrLocationInvalid
	}
	return nil, last
}

// readAvatarThenPhoto reads the public avatar copy, then the canonical
// photo object. An empty public object must not hide the canonical JPEG.
func (c *DfsCore) readAvatarThenPhoto(path string, offset int64, limit int32) ([]byte, error) {
	b, err := c.svcCtx.Dao.GetAvatarFile(c.ctx, path, offset, limit)
	if len(b) > 0 {
		return b, nil
	}
	if err != nil && !errors.Is(err, minio_util.ErrNotFound) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return c.svcCtx.Dao.GetPhotoFile(c.ctx, path, offset, limit)
}
