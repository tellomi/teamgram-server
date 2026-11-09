package catalog

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/objectstore"
	"github.com/teamgram/teamgram-server/pkg/dcoptions"

	"github.com/zeromicro/go-zero/core/logx"
)

var (
	gifCatalogMu sync.RWMutex
	gifCatalog   []gifClip

	gifStoreMu sync.Mutex
	gifStore   *objectstore.Client
)

func setGifCatalog(clips []gifClip) {
	gifCatalogMu.Lock()
	gifCatalog = append([]gifClip(nil), clips...)
	gifCatalogMu.Unlock()
}

func allGifClips() []gifClip {
	gifCatalogMu.RLock()
	extra := append([]gifClip(nil), gifCatalog...)
	gifCatalogMu.RUnlock()
	out := make([]gifClip, 0, len(gifClips)+len(extra))
	out = append(out, gifClips...)
	out = append(out, extra...)
	return out
}

func importedGifByID(id int64) (gifClip, bool) {
	gifCatalogMu.RLock()
	defer gifCatalogMu.RUnlock()
	for _, clip := range gifCatalog {
		if clip.id == id {
			return clip, true
		}
	}
	return gifClip{}, false
}

func gifStorage() *objectstore.Client {
	gifStoreMu.Lock()
	defer gifStoreMu.Unlock()
	if gifStore != nil {
		return gifStore
	}
	client, err := objectstore.Open()
	if err != nil || client == nil {
		return nil
	}
	gifStore = client
	return gifStore
}

func importedGifBytes(clip gifClip) ([]byte, string, bool) {
	if clip.objectPath == "" {
		return nil, "", false
	}
	client := gifStorage()
	if client == nil {
		return nil, "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	limit := int32(clip.size)
	if limit <= 0 || limit > 8<<20 {
		limit = 8 << 20
	}
	body, err := client.Get(ctx, "stickers", clip.objectPath, limit)
	if err != nil || len(body) == 0 {
		return nil, "", false
	}
	mime := clip.mime
	if mime == "" {
		mime = "video/mp4"
	}
	return body, mime, true
}

func importedGifDocument(clip gifClip) *mtproto.Document {
	id := clip.id
	access := clip.access
	if id == 0 {
		id = gifID(clip.code)
	}
	if access == 0 {
		access = gifAccess(clip.code)
	}
	dur := clip.duration
	if dur <= 0 {
		dur = 1
	}
	name := clip.fileName
	if name == "" {
		name = clip.code + ".mp4"
	}
	mime := clip.mime
	if mime == "" {
		mime = "video/mp4"
	}
	w, h := clip.w, clip.h
	if w <= 0 {
		w = 320
	}
	if h <= 0 {
		h = 180
	}
	var thumbs []*mtproto.PhotoSize
	if len(clip.poster) > 0 {
		thumbs = []*mtproto.PhotoSize{
			mtproto.MakeTLPhotoSize(&mtproto.PhotoSize{
				Type:  mtproto.PhotoSZMediumType,
				W:     w,
				H:     h,
				Size2: int32(len(clip.poster)),
			}).To_PhotoSize(),
		}
	}
	attrs := []*mtproto.DocumentAttribute{
		mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{
			FileName: name,
		}).To_DocumentAttribute(),
		mtproto.MakeTLDocumentAttributeAnimated(&mtproto.DocumentAttribute{}).To_DocumentAttribute(),
	}
	var video []*mtproto.VideoSize
	if mime == "video/mp4" {
		attrs = append([]*mtproto.DocumentAttribute{
			mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{
				SupportsStreaming: true,
				Nosound:           true,
				Duration:          dur,
				Duration_FLOAT64:  dur,
				W:                 w,
				H:                 h,
			}).To_DocumentAttribute(),
		}, attrs...)
		if clip.size > 0 {
			video = []*mtproto.VideoSize{
				mtproto.MakeTLVideoSize(&mtproto.VideoSize{
					Type:  "v",
					W:     w,
					H:     h,
					Size2: int32(clip.size),
				}).To_VideoSize(),
			}
		}
	} else {
		attrs = append([]*mtproto.DocumentAttribute{
			mtproto.MakeTLDocumentAttributeImageSize(&mtproto.DocumentAttribute{
				W: w,
				H: h,
			}).To_DocumentAttribute(),
		}, attrs...)
	}
	return mtproto.MakeTLDocument(&mtproto.Document{
		Id:            id,
		AccessHash:    access,
		FileReference: []byte{},
		Date:          stickerDate,
		MimeType:      mime,
		Size2_INT32:   int32(clip.size),
		Size2_INT64:   clip.size,
		Size2:         clip.size,
		Thumbs:        thumbs,
		VideoThumbs:   video,
		DcId:          dcoptions.FileDC(),
		Attributes:    attrs,
	}).To_Document()
}

func (s *Service) loadGifCatalog(ctx context.Context, db *sql.DB) error {
	clips, err := readGifClips(ctx, db)
	if err != nil {
		return err
	}
	fillGifPosters(ctx, clips)
	setGifCatalog(clips)
	v, err := catalogVersion(ctx, db, catalogGifs)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.gifVersion = v
	s.gifVersionLoaded = true
	s.mu.Unlock()
	return nil
}

// ReloadGifs re-reads the imported GIF catalog when its version row changes.
func (s *Service) ReloadGifs(ctx context.Context) error {
	mp, ok := s.mysql()
	if !ok {
		return nil
	}
	v, err := catalogVersion(ctx, mp.db, catalogGifs)
	if err != nil {
		return err
	}
	s.mu.Lock()
	same := s.gifVersionLoaded && s.gifVersion == v
	s.mu.Unlock()
	if same {
		return nil
	}
	return s.loadGifCatalog(ctx, mp.db)
}

func readGifClips(ctx context.Context, db *sql.DB) ([]gifClip, error) {
	rows, err := db.QueryContext(ctx, `
SELECT id, access_hash, code, title, keywords, mime_type, file_size, width, height, duration, file_name, object_path, thumb_path
FROM tellomi_gif_clips ORDER BY sort_order, id`)
	if err != nil {
		if isMissingTable(err, "tellomi_gif_clips") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []gifClip
	for rows.Next() {
		var clip gifClip
		var keywords string
		if err = rows.Scan(&clip.id, &clip.access, &clip.code, &clip.title, &keywords, &clip.mime, &clip.size, &clip.w, &clip.h, &clip.duration, &clip.fileName, &clip.objectPath, &clip.thumbPath); err != nil {
			return nil, err
		}
		clip.imported = true
		clip.keywords = splitStoredKeywords(keywords)
		out = append(out, clip)
	}
	return out, rows.Err()
}

func splitStoredKeywords(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "\n")
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func joinStoredKeywords(words []string) string {
	return strings.Join(words, "\n")
}

func fillGifPosters(ctx context.Context, clips []gifClip) {
	need := false
	for _, clip := range clips {
		if clip.thumbPath != "" {
			need = true
			break
		}
	}
	if !need {
		return
	}
	client := gifStorage()
	if client == nil {
		return
	}
	for i := range clips {
		if clips[i].thumbPath == "" {
			continue
		}
		body, err := client.Get(ctx, "stickers", clips[i].thumbPath, 1<<20)
		if err != nil || len(body) == 0 {
			logx.Errorf("gif poster %s: %v", clips[i].thumbPath, err)
			continue
		}
		clips[i].poster = body
	}
}

func gifObjectPath(id int64) string {
	return strconv.FormatInt(id, 10) + ".dat"
}

func gifThumbPath(id int64) string {
	return strconv.FormatInt(id, 10) + ".jpg"
}
