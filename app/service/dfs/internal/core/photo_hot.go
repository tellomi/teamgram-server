package core

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/minio_util"
)

// sizePut is one object that must be durable before the photo RPC returns.
type sizePut struct {
	path string
	body []byte
}

// photoSizePuts is the set of keys written for one generated size.
//
// shared means the bytes already belong to an earlier upload's key
// (proposed). That key is not written again: a fresh encode is not
// guaranteed to match the object the first upload stored.
//
// A size whose bytes are the original file is recorded on the original
// key. With the hot tier on, the conventional <type>/<id>.dat key is
// written as well: download uses that key when the path row is missing.
func photoSizePuts(photoID int64, szType, proposed string, body, original []byte, claimPath string, shared, hot bool) (record string, puts []sizePut) {
	record = proposed
	if shared {
		return record, nil
	}
	aliased := len(original) > 0 && bytes.Equal(body, original) && claimPath != ""
	if aliased {
		record = claimPath
	}
	add := func(path string, b []byte) {
		if path == "" || len(b) == 0 {
			return
		}
		for _, have := range puts {
			if have.path == path {
				return
			}
		}
		puts = append(puts, sizePut{path: path, body: b})
	}
	if !aliased {
		add(proposed, body)
	}
	if hot {
		add(fmt.Sprintf("%s/%d.dat", szType, photoID), body)
	}
	return record, puts
}

// photoDownloadPaths is the hot-tier lookup order for one photo size.
// The recorded key is first. The conventional key is the fallback download
// uses when that row is empty.
func photoDownloadPaths(primary, size string, id int64) []string {
	var out []string
	add := func(p string) {
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
	add(primary)
	if size != "" && id != 0 {
		add(fmt.Sprintf("%s/%d.dat", size, id))
	}
	return out
}

func (c *DfsCore) readHotPaths(class string, paths []string, offset int64, limit int32) ([]byte, bool) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MinioUtil == nil {
		return nil, false
	}
	for _, p := range paths {
		b, ok := c.svcCtx.Dao.ReadHot(class, p, offset, limit)
		if !ok {
			continue
		}
		if len(b) > 0 || offset > 0 || limit == 0 {
			return b, true
		}
	}
	return nil, false
}

// readStoredPhoto serves a photo size from the hot tier when any candidate
// key is local. Object storage is used only after that miss. A timeout
// there is returned as-is: the avatar bucket is not a second place to wait
// out a slow R2 read. A definitive miss still checks the avatar copy.
func (c *DfsCore) readStoredPhoto(id int64, size string, offset int64, limit int32, remote func(path string) ([]byte, error)) ([]byte, error) {
	primary := ""
	if c != nil && c.svcCtx != nil && c.svcCtx.Dao != nil {
		primary = c.svcCtx.Dao.PhotoObjectPath(c.ctx, id, size)
	}
	paths := photoDownloadPaths(primary, size, id)
	classes := []string{minio_util.ClassPhotos}
	if mtproto.PhotoSizeIsVideo(size) {
		classes = []string{minio_util.ClassVideos, minio_util.ClassPhotos}
	}
	for _, class := range classes {
		if b, ok := c.readHotPaths(class, paths, offset, limit); ok {
			return b, nil
		}
	}
	if primary == "" && len(paths) > 0 {
		primary = paths[0]
	}
	if remote == nil {
		return nil, io.EOF
	}
	b, err := remote(primary)
	if errors.Is(err, io.EOF) {
		if b == nil {
			b = []byte{}
		}
		return b, nil
	}
	if len(b) > 0 || (err == nil && (offset > 0 || limit == 0)) {
		return b, nil
	}
	if err != nil && !errors.Is(err, minio_util.ErrNotFound) {
		return nil, err
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return b, err
	}
	return c.svcCtx.Dao.GetAvatarFile(c.ctx, primary, offset, limit)
}
