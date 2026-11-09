package catalog

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/teamgram/teamgram-server/app/service/dfs/objectstore"
)

const (
	gifManifestName = "manifest.json"
	maxGifBytes     = 10 << 20
	maxGifTitle     = 64
)

// GifImportOptions controls importgifs.
type GifImportOptions struct {
	DryRun bool
}

type gifManifest struct {
	Clips []gifManifestClip `json:"clips"`
}

type gifManifestClip struct {
	File     string          `json:"file"`
	Title    string          `json:"title"`
	Keywords json.RawMessage `json:"keywords"`
	Width    int32           `json:"width"`
	Height   int32           `json:"height"`
	Duration float64         `json:"duration"`
}

type preparedGif struct {
	clip       gifClip
	bodyPath   string
	posterPath string
}

// ReadGifDir validates a directory of mp4/gif clips.
// A manifest.json lists the clips. Without one, each mp4 or gif in the
// directory is a clip, and an optional <name>.json supplies title, keywords,
// width, height, and duration.
func ReadGifDir(dir string) ([]preparedGif, error) {
	dir = filepath.Clean(dir)
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	manifestPath := filepath.Join(dir, gifManifestName)
	if _, err = os.Stat(manifestPath); err == nil {
		return readGifManifest(dir, manifestPath)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return readGifFiles(dir)
}

func readGifManifest(dir, path string) ([]preparedGif, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest gifManifest
	if err = json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("%s: %w", gifManifestName, err)
	}
	if len(manifest.Clips) == 0 {
		return nil, fmt.Errorf("%s has no clips", gifManifestName)
	}
	var out []preparedGif
	seen := map[string]bool{}
	for i, clip := range manifest.Clips {
		prepared, err := prepareGifClip(dir, clip, i)
		if err != nil {
			return nil, fmt.Errorf("clip %d: %w", i+1, err)
		}
		if seen[prepared.clip.code] {
			return nil, fmt.Errorf("duplicate file %s", clip.File)
		}
		seen[prepared.clip.code] = true
		out = append(out, prepared)
	}
	return out, nil
}

func readGifFiles(dir string) ([]preparedGif, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".mp4", ".gif":
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("%s has no %s and no mp4 or gif files", dir, gifManifestName)
	}
	var out []preparedGif
	for i, name := range names {
		clip := gifManifestClip{File: name}
		sidecar := filepath.Join(dir, strings.TrimSuffix(name, filepath.Ext(name))+".json")
		if body, err := os.ReadFile(sidecar); err == nil {
			if err = json.Unmarshal(body, &clip); err != nil {
				return nil, fmt.Errorf("%s: %w", filepath.Base(sidecar), err)
			}
			clip.File = name
		}
		prepared, err := prepareGifClip(dir, clip, i)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		out = append(out, prepared)
	}
	return out, nil
}

func prepareGifClip(dir string, in gifManifestClip, order int) (preparedGif, error) {
	name := filepath.Base(strings.TrimSpace(in.File))
	if name == "" || name == "." || strings.Contains(name, "..") {
		return preparedGif{}, fmt.Errorf("file is required")
	}
	ext := strings.ToLower(filepath.Ext(name))
	var mime string
	switch ext {
	case ".mp4":
		mime = "video/mp4"
	case ".gif":
		mime = "image/gif"
	default:
		return preparedGif{}, fmt.Errorf("%s must be .mp4 or .gif", name)
	}
	code := strings.TrimSuffix(name, filepath.Ext(name))
	if !gifCodeRE.MatchString(code) {
		return preparedGif{}, fmt.Errorf("file name %s must match %s so the document id stays stable", name, gifCodeRE.String())
	}
	if builtinGifCode(code) {
		return preparedGif{}, fmt.Errorf("%s is a built-in gif", code)
	}
	bodyPath := filepath.Join(dir, name)
	info, err := os.Stat(bodyPath)
	if err != nil {
		return preparedGif{}, err
	}
	if info.Size() <= 0 || info.Size() > maxGifBytes {
		return preparedGif{}, fmt.Errorf("%s is %d bytes; the limit is %d", name, info.Size(), maxGifBytes)
	}
	words, langs, err := parseManifestKeywords(in.Keywords)
	if err != nil {
		return preparedGif{}, err
	}
	keywords, err := cleanKeywordList(flattenGifKeywords(words, langs))
	if err != nil {
		return preparedGif{}, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = code
	}
	if utf8.RuneCountInString(title) > maxGifTitle {
		return preparedGif{}, fmt.Errorf("title is longer than %d characters", maxGifTitle)
	}
	w, h := in.Width, in.Height
	if w <= 0 || h <= 0 {
		if dw, dh, ok := imageSize(bodyPath); ok {
			if w <= 0 {
				w = dw
			}
			if h <= 0 {
				h = dh
			}
		}
	}
	if w <= 0 {
		w = 320
	}
	if h <= 0 {
		h = 180
	}
	dur := in.Duration
	if dur <= 0 {
		dur = 1
	}
	id := gifID(code)
	access := gifAccess(code)
	posterPath := ""
	for _, ext := range []string{".jpg", ".jpeg"} {
		candidate := filepath.Join(dir, code+ext)
		if st, err := os.Stat(candidate); err == nil && st.Size() > 0 && st.Size() <= 1<<20 {
			posterPath = candidate
			break
		}
	}
	thumb := ""
	if posterPath != "" {
		thumb = gifThumbPath(id)
	}
	return preparedGif{
		bodyPath:   bodyPath,
		posterPath: posterPath,
		clip: gifClip{
			code:       code,
			title:      title,
			w:          w,
			h:          h,
			keywords:   keywords,
			mime:       mime,
			size:       info.Size(),
			duration:   dur,
			fileName:   name,
			objectPath: gifObjectPath(id),
			thumbPath:  thumb,
			imported:   true,
			id:         id,
			access:     access,
		},
	}, nil
}

var gifCodeRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

func builtinGifCode(code string) bool {
	for _, clip := range gifClips {
		if clip.code == code {
			return true
		}
	}
	return false
}

func flattenGifKeywords(words []string, langs map[string][]string) []string {
	out := append([]string{}, words...)
	keys := make([]string, 0, len(langs))
	for key := range langs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out = append(out, langs[key]...)
	}
	return out
}

func imageSize(path string) (int32, int32, bool) {
	body, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, false
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, false
	}
	return int32(cfg.Width), int32(cfg.Height), true
}

// ImportGifs upserts a directory of GIF clips and uploads their bytes with
// the dfs S3 client. -dry-run validates and prints the plan.
func ImportGifs(dsn, dir string, opt GifImportOptions) error {
	clips, err := ReadGifDir(dir)
	if err != nil {
		return err
	}
	for _, clip := range clips {
		log.Printf("importgifs: %s title=%s keywords=%d %dx%d %s",
			clip.clip.fileName, clip.clip.title, len(clip.clip.keywords), clip.clip.w, clip.clip.h, clip.clip.mime)
	}
	mode := "import"
	if opt.DryRun {
		mode = "dry-run"
	}
	log.Printf("importgifs: %s %d clips", mode, len(clips))
	if opt.DryRun {
		return nil
	}
	if env := os.Getenv("TEAMGRAM_MYSQL_DSN"); env != "" {
		dsn = env
	}
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return fmt.Errorf("TEAMGRAM_MYSQL_DSN or -dsn is required")
	}
	client, err := objectstore.Open()
	if err != nil {
		return err
	}
	if client == nil {
		return fmt.Errorf("object storage is not configured (TELLOMI_S3_*)")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		return err
	}
	if err = execSchema(ctx, db); err != nil {
		return err
	}
	if err = uploadGifClips(ctx, client, clips); err != nil {
		return err
	}
	if err = upsertGifClips(ctx, db, clips); err != nil {
		return err
	}
	if err = bumpCatalogVersion(ctx, db, catalogGifs); err != nil {
		return err
	}
	log.Printf("importgifs: wrote %d clips", len(clips))
	return nil
}

func uploadGifClips(ctx context.Context, client *objectstore.Client, clips []preparedGif) error {
	for _, clip := range clips {
		body, err := os.ReadFile(clip.bodyPath)
		if err != nil {
			return err
		}
		if err = putGifObject(ctx, client, clip.clip.objectPath, clip.clip.mime, body); err != nil {
			return fmt.Errorf("upload %s: %w", clip.clip.fileName, err)
		}
		if clip.posterPath == "" {
			continue
		}
		poster, err := os.ReadFile(clip.posterPath)
		if err != nil {
			return err
		}
		if err = putGifObject(ctx, client, clip.clip.thumbPath, "image/jpeg", poster); err != nil {
			return fmt.Errorf("upload %s: %w", filepath.Base(clip.posterPath), err)
		}
	}
	return nil
}

func putGifObject(ctx context.Context, client *objectstore.Client, path, mime string, body []byte) error {
	size, err := client.Size(ctx, "stickers", path)
	if err == nil && size == int64(len(body)) {
		return nil
	}
	if err != nil && !errors.Is(err, objectstore.ErrNotFound) {
		return err
	}
	return client.Put(ctx, "stickers", path, mime, body)
}

func upsertGifClips(ctx context.Context, db *sql.DB, clips []preparedGif) error {
	for i, clip := range clips {
		c := clip.clip
		if _, err := db.ExecContext(ctx, `
INSERT INTO tellomi_gif_clips
  (id, access_hash, code, title, keywords, mime_type, file_size, width, height, duration, file_name, object_path, thumb_path, sort_order)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  access_hash=VALUES(access_hash), title=VALUES(title), keywords=VALUES(keywords),
  mime_type=VALUES(mime_type), file_size=VALUES(file_size), width=VALUES(width), height=VALUES(height),
  duration=VALUES(duration), file_name=VALUES(file_name), object_path=VALUES(object_path),
  thumb_path=VALUES(thumb_path), sort_order=VALUES(sort_order)`,
			c.id, c.access, c.code, c.title, joinStoredKeywords(c.keywords), c.mime, c.size, c.w, c.h, c.duration,
			c.fileName, c.objectPath, c.thumbPath, i); err != nil {
			return err
		}
	}
	return nil
}
