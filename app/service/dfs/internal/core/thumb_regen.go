package core

import (
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/minio_util"
	"github.com/teamgram/teamgram-server/app/service/dfs/thumbbackfill"
)

// maxThumbSource is the largest document an on-demand thumb rebuild reads.
// Larger videos are left to the backfill command, which streams to disk.
const maxThumbSource int32 = 32 << 20

const thumbRegenCooldown = 30 * time.Second

var thumbRegenFail struct {
	sync.Mutex
	until map[string]time.Time
}

func (c *DfsCore) readPublicDocument(class, path string, offset int64, limit int32) ([]byte, error) {
	switch class {
	case minio_util.ClassStickers:
		return c.svcCtx.Dao.GetStickerFile(c.ctx, path, offset, limit)
	case minio_util.ClassEmoji:
		return c.svcCtx.Dao.GetEmojiFile(c.ctx, path, offset, limit)
	default:
		return nil, fmt.Errorf("catalog class %q", class)
	}
}

func (c *DfsCore) finishDocumentThumb(location *mtproto.InputFileLocation, offset int64, limit int32, bytes []byte, readErr error) ([]byte, error) {
	var regen func() ([]byte, error)
	if location != nil && !mtproto.PhotoSizeIsVideo(location.GetThumbSize()) {
		regen = func() ([]byte, error) {
			return c.regenerateDocumentThumb(location.GetId(), location.GetThumbSize())
		}
	}
	return resolveThumbDownload(offset, limit, bytes, readErr, regen)
}

// resolveThumbDownload turns a missing still thumb into a rebuilt JPEG or
// FILE_ID_INVALID. An empty upload.file is a successful download, and the
// official clients retry it. A network error is returned as-is.
func resolveThumbDownload(offset int64, limit int32, bytes []byte, readErr error, regen func() ([]byte, error)) ([]byte, error) {
	if readErr == nil && (limit == 0 || len(bytes) > 0 || offset > 0) {
		if bytes == nil {
			bytes = []byte{}
		}
		return bytes, nil
	}
	if readErr != nil && !errors.Is(readErr, minio_util.ErrNotFound) {
		return nil, readErr
	}
	if regen != nil {
		if body, err := regen(); err == nil && len(body) > 0 {
			return sliceRange(body, offset, limit), nil
		}
	}
	return nil, mtproto.ErrFileIdInvalid
}

func sliceRange(body []byte, offset int64, limit int32) []byte {
	if offset < 0 || offset >= int64(len(body)) {
		return []byte{}
	}
	rest := body[offset:]
	if limit > 0 && int(limit) < len(rest) {
		rest = rest[:limit]
	}
	out := make([]byte, len(rest))
	copy(out, rest)
	return out
}

func (c *DfsCore) regenerateDocumentThumb(id int64, size string) ([]byte, error) {
	if id == 0 || size == "" || mtproto.PhotoSizeIsVideo(size) {
		return nil, errors.New("thumb cannot be rebuilt")
	}
	if thumbRegenSkipped(id, size) {
		return nil, errors.New("thumb rebuild cooling down")
	}
	body, err := c.rebuildThumbJPEG(id)
	if err != nil || len(body) == 0 {
		markThumbRegenFailed(id, size)
		if err == nil {
			err = errors.New("empty thumb")
		}
		return nil, err
	}
	if c != nil && c.svcCtx != nil && c.svcCtx.Dao != nil {
		path := c.svcCtx.Dao.PhotoObjectPath(c.ctx, id, size)
		if path == "" {
			path = fmt.Sprintf("%s/%d.dat", size, id)
		}
		if _, err = c.svcCtx.Dao.PutPhotoFile(c.ctx, path, body); err != nil {
			c.Logger.Errorf("dfs.regenerateThumb %s: %v", path, err)
			markThumbRegenFailed(id, size)
			return nil, err
		}
	}
	return body, nil
}

func (c *DfsCore) rebuildThumbJPEG(id int64) ([]byte, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.FFmpegUtil == nil {
		return nil, errors.New("thumb rebuild unavailable")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, err
	}
	path := c.svcCtx.Dao.DocumentObjectPath(c.ctx, id)
	src, err := c.svcCtx.Dao.GetDocumentFile(c.ctx, path, 0, maxThumbSource+1)
	if err != nil || len(src) == 0 || len(src) > int(maxThumbSource) {
		if err == nil {
			err = errors.New("document is missing or too large to rebuild a thumb")
		}
		return nil, err
	}
	frame, err := c.svcCtx.FFmpegUtil.GetFirstFrameByPipe(src)
	if err != nil || len(frame) == 0 {
		if err == nil {
			err = errors.New("ffmpeg produced no frame")
		}
		return nil, err
	}
	return thumbbackfill.FitJPEG(frame)
}

func thumbRegenKey(id int64, size string) string {
	return fmt.Sprintf("%d/%s", id, size)
}

func thumbRegenSkipped(id int64, size string) bool {
	thumbRegenFail.Lock()
	defer thumbRegenFail.Unlock()
	until, ok := thumbRegenFail.until[thumbRegenKey(id, size)]
	if !ok {
		return false
	}
	if !time.Now().Before(until) {
		delete(thumbRegenFail.until, thumbRegenKey(id, size))
		return false
	}
	return true
}

func markThumbRegenFailed(id int64, size string) {
	thumbRegenFail.Lock()
	defer thumbRegenFail.Unlock()
	if thumbRegenFail.until == nil {
		thumbRegenFail.until = map[string]time.Time{}
	}
	thumbRegenFail.until[thumbRegenKey(id, size)] = time.Now().Add(thumbRegenCooldown)
}
