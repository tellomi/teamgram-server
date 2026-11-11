package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "image/gif"
	_ "image/png"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/minio_util"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/model"
	"github.com/teamgram/teamgram-server/app/service/dfs/thumbbackfill"
)

// maxThumbSource is the largest document an on-demand thumb rebuild holds
// in memory. Larger videos are streamed to a temp file, up to maxThumbStream.
const maxThumbSource int32 = 32 << 20

// maxThumbStream is the largest video an on-demand rebuild will spool to
// disk. Above that the thumb is not invented: the caller returns
// FILE_ID_INVALID.
const maxThumbStream int64 = 256 << 20

// errThumbPermanent means the bytes are not a still image or a video frame
// this process can turn into the JPEG the client asked for. A storage or
// network error must not use this sentinel: callers return that error
// unchanged instead of FILE_ID_INVALID.
var errThumbPermanent = errors.New("thumb cannot be rebuilt")

const thumbRegenCooldown = 30 * time.Second

// A rebuild uses its own ffmpeg process. Two may run at once, and their
// temp files share a 512MiB disk budget. A third waits until the request
// context ends instead of starting another unbounded job.
const (
	maxThumbRebuilds = 2
	maxThumbDisk     = 2 * maxThumbStream
	thumbFFmpegLimit = 20 * time.Second
)

var (
	thumbSlots     = make(chan struct{}, maxThumbRebuilds)
	thumbDisk      atomic.Int64
	runThumbFFmpeg = defaultThumbFFmpeg
)

func acquireThumbSlot(ctx context.Context, disk int64) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if disk < 1 {
		disk = 1
	}
	if !reserveThumbDisk(disk) {
		return nil, errors.New("thumb rebuild disk budget")
	}
	select {
	case thumbSlots <- struct{}{}:
		return func() {
			<-thumbSlots
			releaseThumbDisk(disk)
		}, nil
	case <-ctx.Done():
		releaseThumbDisk(disk)
		return nil, ctx.Err()
	}
}

func reserveThumbDisk(n int64) bool {
	for {
		cur := thumbDisk.Load()
		if cur+n > maxThumbDisk {
			return false
		}
		if thumbDisk.CompareAndSwap(cur, cur+n) {
			return true
		}
	}
}

func releaseThumbDisk(n int64) {
	thumbDisk.Add(-n)
}

func defaultThumbFFmpeg(ctx context.Context, path string) ([]byte, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("%w: %v", errThumbPermanent, err)
	}
	ctx, cancel := context.WithTimeout(ctx, thumbFFmpegLimit)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-nostdin", "-nostats", "-loglevel", "error",
		"-i", path,
		"-frames:v", "1",
		"-f", "image2pipe", "-vcodec", "mjpeg",
		"pipe:1")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: ffmpeg", errThumbPermanent)
	}
	if stdout.Len() == 0 {
		return nil, fmt.Errorf("%w: ffmpeg produced no frame", errThumbPermanent)
	}
	return stdout.Bytes(), nil
}

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
		body, err := regen()
		if err == nil && len(body) > 0 {
			return sliceRange(body, offset, limit), nil
		}
		// A timeout or a refused write is not an invalid file id. The
		// client retries those. FILE_ID_INVALID stops the download.
		if err != nil && !errors.Is(err, errThumbPermanent) {
			return nil, err
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
		return nil, errThumbPermanent
	}
	if thumbRegenSkipped(id, size) {
		return nil, errThumbPermanent
	}
	body, err := c.rebuildThumbJPEG(id)
	if err != nil || len(body) == 0 {
		if err == nil || errors.Is(err, errThumbPermanent) {
			markThumbRegenFailed(id, size)
			if err == nil {
				err = errThumbPermanent
			}
		}
		return nil, err
	}
	if c != nil && c.svcCtx != nil && c.svcCtx.Dao != nil {
		path := c.svcCtx.Dao.PhotoObjectPath(c.ctx, id, size)
		if path == "" {
			path = fmt.Sprintf("%s/%d.dat", size, id)
		}
		if _, putErr := c.svcCtx.Dao.PutPhotoFile(c.ctx, path, body); putErr != nil {
			// The JPEG is already in hand. A failed cache write must not
			// become FILE_ID_INVALID for this download.
			c.Logger.Errorf("dfs.regenerateThumb put: %v", putErr)
		}
	}
	return body, nil
}

func (c *DfsCore) rebuildThumbJPEG(id int64) ([]byte, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil, fmt.Errorf("%w: unavailable", errThumbPermanent)
	}
	path := c.svcCtx.Dao.DocumentObjectPath(c.ctx, id)
	n, statErr := c.svcCtx.Dao.Stat(c.ctx, minio_util.ClassDocuments, path)
	if statErr != nil && !errors.Is(statErr, minio_util.ErrNotFound) {
		return nil, statErr
	}
	if statErr == nil && n > maxThumbStream {
		return nil, fmt.Errorf("%w: document is too large", errThumbPermanent)
	}
	if statErr == nil && n > int64(maxThumbSource) {
		return c.frameFromStoredDocument(path)
	}
	var src []byte
	var err error
	if statErr == nil {
		src, err = c.svcCtx.Dao.GetDocumentFile(c.ctx, path, 0, maxThumbSource+1)
		if err != nil {
			return nil, err
		}
	} else {
		src, err = c.thumbSourceFallback(id, path)
		if err != nil {
			return nil, err
		}
	}
	if len(src) == 0 {
		return nil, fmt.Errorf("%w: document missing", errThumbPermanent)
	}
	if len(src) > int(maxThumbSource) {
		return nil, fmt.Errorf("%w: document is too large", errThumbPermanent)
	}
	if jpg, ok := thumbFromDocumentBytes(src); ok {
		return jpg, nil
	}
	switch stillKind(src) {
	case "jpeg", "png", "gif":
		return nil, fmt.Errorf("%w: still image did not decode", errThumbPermanent)
	}
	return c.jpegFromVideoBytes(src)
}

func (c *DfsCore) jpegFromVideoBytes(src []byte) ([]byte, error) {
	ctx := context.Background()
	if c != nil && c.ctx != nil {
		ctx = c.ctx
	}
	release, err := acquireThumbSlot(ctx, maxThumbStream)
	if err != nil {
		return nil, err
	}
	defer release()
	f, err := os.CreateTemp("", "tellomi-thumb-*")
	if err != nil {
		return nil, err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(src); err != nil {
		f.Close()
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	return finishVideoFrame(ctx, name)
}

func (c *DfsCore) frameFromStoredDocument(path string) ([]byte, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil, fmt.Errorf("%w: ffmpeg unavailable", errThumbPermanent)
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	release, err := acquireThumbSlot(ctx, maxThumbStream)
	if err != nil {
		return nil, err
	}
	defer release()
	obj, err := c.svcCtx.Dao.GetFileObject(ctx, minio_util.ClassDocuments, path)
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	f, err := os.CreateTemp("", "tellomi-thumb-*")
	if err != nil {
		return nil, err
	}
	name := f.Name()
	defer os.Remove(name)
	written, err := copyThumb(ctx, f, io.LimitReader(obj, maxThumbStream+1))
	if err != nil {
		f.Close()
		return nil, err
	}
	if written > maxThumbStream {
		f.Close()
		return nil, fmt.Errorf("%w: document is too large", errThumbPermanent)
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	return finishVideoFrame(ctx, name)
}

func finishVideoFrame(ctx context.Context, path string) ([]byte, error) {
	frame, err := runThumbFFmpeg(ctx, path)
	if err != nil || len(frame) == 0 {
		if err != nil && !errors.Is(err, errThumbPermanent) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: ffmpeg produced no frame", errThumbPermanent)
	}
	jpg, err := thumbbackfill.FitJPEG(frame)
	if err != nil || !jpegDecodable(jpg) {
		return nil, fmt.Errorf("%w: frame is not a jpeg", errThumbPermanent)
	}
	return jpg, nil
}

func copyThumb(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var n int64
	for {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[:nr])
			n += int64(nw)
			if ew != nil {
				return n, ew
			}
		}
		if er != nil {
			if errors.Is(er, io.EOF) {
				return n, nil
			}
			return n, er
		}
	}
}

func (c *DfsCore) thumbSourceFallback(id int64, docPath string) ([]byte, error) {
	b, err := c.svcCtx.Dao.GetCacheFile(c.ctx, id, 0, maxThumbSource+1, docPath, c.svcCtx.Dao.GetDocumentFile)
	if err == nil && len(b) > 0 {
		if len(b) > int(maxThumbSource) {
			return nil, fmt.Errorf("%w: cached document too large", errThumbPermanent)
		}
		return b, nil
	}
	if err != nil && !errors.Is(err, minio_util.ErrNotFound) && !errors.Is(err, model.ErrorDfsFileNotFound) {
		return nil, err
	}
	if class, key, ok := catalog.PublicObject(id); ok {
		b, err = c.readPublicDocument(class, key, 0, maxThumbSource+1)
		if err != nil {
			if errors.Is(err, minio_util.ErrNotFound) {
				return nil, fmt.Errorf("%w: document missing", errThumbPermanent)
			}
			return nil, err
		}
		if len(b) > int(maxThumbSource) {
			return nil, fmt.Errorf("%w: public document too large", errThumbPermanent)
		}
		if len(b) == 0 {
			return nil, fmt.Errorf("%w: document missing", errThumbPermanent)
		}
		return b, nil
	}
	return nil, fmt.Errorf("%w: document missing", errThumbPermanent)
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

type thumbRead func(ctx context.Context, path string, offset int64, limit int32) ([]byte, error)

// readDocumentThumb tries each candidate key. An empty object at offset 0
// is not a hit: the JPEG may live on the deduped origin key. A storage
// error is kept when no candidate has bytes.
func (c *DfsCore) readDocumentThumb(offset int64, limit int32, paths []string, primary, fallback thumbRead) ([]byte, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || primary == nil {
		return nil, fmt.Errorf("object storage client is nil")
	}
	reads := make([]func() ([]byte, error), 0, len(paths)+1)
	for _, path := range paths {
		path := path
		reads = append(reads, func() ([]byte, error) {
			return primary(c.ctx, path, offset, limit)
		})
	}
	if fallback != nil && len(paths) > 0 {
		path := paths[0]
		reads = append(reads, func() ([]byte, error) {
			return fallback(c.ctx, path, offset, limit)
		})
	}
	return readThumbCandidates(offset, reads...)
}

func readThumbCandidates(offset int64, reads ...func() ([]byte, error)) ([]byte, error) {
	var hard, last error
	for _, read := range reads {
		b, err := read()
		if err == nil && len(b) > 0 {
			return b, nil
		}
		if errors.Is(err, io.EOF) {
			if offset > 0 || len(b) > 0 {
				if b == nil {
					b = []byte{}
				}
				return b, nil
			}
			if last == nil {
				last = minio_util.ErrNotFound
			}
			continue
		}
		if err != nil && !errors.Is(err, minio_util.ErrNotFound) {
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

// documentThumbPaths lists photo keys for one document size. The recorded
// media_photo_paths key is first. The conventional <type>/<id>.dat key and
// the deduped origin's key follow, so a reused document still finds the
// JPEG the first upload wrote.
func (c *DfsCore) documentThumbPaths(id int64, size string) []string {
	recorded := ""
	var extra []string
	var origin int64
	if c != nil && c.svcCtx != nil && c.svcCtx.Dao != nil {
		recorded = c.svcCtx.Dao.PhotoObjectPath(c.ctx, id, size)
		extra = append(extra, c.photoSizePaths(id, size)...)
		origin = originIDFromDocumentPath(c.svcCtx.Dao.DocumentObjectPath(c.ctx, id), id)
		if origin != 0 {
			if p := c.svcCtx.Dao.PhotoObjectPath(c.ctx, origin, size); p != "" {
				extra = append(extra, p)
			}
			extra = append(extra, c.photoSizePaths(origin, size)...)
		}
	}
	return thumbPathList(size, id, recorded, extra, origin)
}

func (c *DfsCore) photoSizePaths(id int64, size string) []string {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Blobs == nil || id == 0 || size == "" {
		return nil
	}
	rows, err := c.svcCtx.Dao.Blobs.ListPhotoSizes(c.ctx, id)
	if err != nil {
		c.Logger.Errorf("dfs.photoSizePaths: %v", err)
		return nil
	}
	var out []string
	for _, row := range rows {
		if row.Type != size || row.Path == "" {
			continue
		}
		switch row.CachedType {
		case cachedTypePathSize, cachedTypeStrippedSize, cachedTypeCachedSize:
			continue
		}
		out = append(out, row.Path)
	}
	return out
}

func thumbPathList(size string, id int64, recorded string, extra []string, originID int64) []string {
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		for _, have := range out {
			if have == p {
				return
			}
		}
		out = append(out, p)
	}
	add(recorded)
	if size != "" && id != 0 {
		add(fmt.Sprintf("%s/%d.dat", size, id))
	}
	for _, p := range extra {
		add(p)
	}
	if originID != 0 && originID != id && size != "" {
		add(fmt.Sprintf("%s/%d.dat", size, originID))
	}
	return out
}

func originIDFromDocumentPath(path string, self int64) int64 {
	path = strings.TrimSpace(path)
	if path == "" {
		return 0
	}
	base := path
	if i := strings.LastIndex(path, "/"); i >= 0 {
		base = path[i+1:]
	}
	if !strings.HasSuffix(base, ".dat") {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSuffix(base, ".dat"), 10, 64)
	if err != nil || n <= 0 || n == self {
		return 0
	}
	return n
}

func stillKind(b []byte) string {
	if len(b) >= 3 && b[0] == 0xff && b[1] == 0xd8 && b[2] == 0xff {
		return "jpeg"
	}
	if len(b) >= 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n" {
		return "png"
	}
	if len(b) >= 6 && (string(b[:6]) == "GIF87a" || string(b[:6]) == "GIF89a") {
		return "gif"
	}
	if len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP" {
		return "webp"
	}
	return ""
}

// thumbFromDocumentBytes resizes a JPEG, PNG, or GIF document into the
// medium thumb clients download. WebP and video go through ffmpeg. An
// arbitrary file is not a thumbnail.
func thumbFromDocumentBytes(src []byte) ([]byte, bool) {
	switch stillKind(src) {
	case "jpeg", "png", "gif":
	default:
		return nil, false
	}
	if _, _, err := image.Decode(bytes.NewReader(src)); err != nil {
		return nil, false
	}
	jpg, err := thumbbackfill.FitJPEG(src)
	if err != nil || !jpegDecodable(jpg) {
		return nil, false
	}
	return jpg, true
}

func jpegDecodable(b []byte) bool {
	if len(b) < 4 || b[0] != 0xff || b[1] != 0xd8 {
		return false
	}
	_, err := jpeg.Decode(bytes.NewReader(b))
	return err == nil
}
