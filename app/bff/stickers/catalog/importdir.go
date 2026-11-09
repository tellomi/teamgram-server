package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/teamgram/teamgram-server/app/service/dfs/objectstore"
)

const (
	manifestName       = "manifest.json"
	maxStickersPerPack = 20000
)

// ImportOptions controls a prepared-pack import.
// Featured and InstalledDefault force those flags on for every pack in the
// directory. A manifest can also set them on its own.
type ImportOptions struct {
	DryRun           bool
	Featured         bool
	InstalledDefault bool
}

type packManifest struct {
	Title            string         `json:"title"`
	ShortName        string         `json:"short_name"`
	Type             string         `json:"type"`
	Featured         bool           `json:"featured"`
	InstalledDefault bool           `json:"installed_default"`
	License          string         `json:"license"`
	Attribution      string         `json:"attribution"`
	SourceURL        string         `json:"source_url"`
	Stickers         []manifestFile `json:"stickers"`
}

type manifestFile struct {
	File     string          `json:"file"`
	Emoticon string          `json:"emoticon"`
	Keywords json.RawMessage `json:"keywords"`
}

// ReadPrepared loads and validates a directory of sticker packs.
// The directory is one pack when it contains manifest.json. Otherwise each
// immediate child directory that contains manifest.json is a pack.
// Paths maps a document id to the file that should be uploaded. Dry-run
// callers can ignore it.
func ReadPrepared(dir string, opt ImportOptions) ([]*Pack, map[int64]string, error) {
	dir = filepath.Clean(dir)
	info, err := os.Stat(dir)
	if err != nil {
		return nil, nil, err
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("%s is not a directory", dir)
	}
	roots, err := packRoots(dir)
	if err != nil {
		return nil, nil, err
	}
	if len(roots) == 0 {
		return nil, nil, fmt.Errorf("%s has no %s", dir, manifestName)
	}
	var packs []*Pack
	paths := map[int64]string{}
	shorts := map[string]string{}
	for _, root := range roots {
		part, partPaths, err := readPack(root, opt)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", root, err)
		}
		for _, p := range part {
			key := strings.ToLower(p.ShortName)
			if prev, ok := shorts[key]; ok {
				return nil, nil, fmt.Errorf("short_name %s is used by both %s and %s", p.ShortName, prev, root)
			}
			shorts[key] = root
			packs = append(packs, p)
		}
		for id, path := range partPaths {
			if prev, ok := paths[id]; ok {
				return nil, nil, fmt.Errorf("document %d is both %s and %s", id, prev, path)
			}
			paths[id] = path
		}
	}
	return packs, paths, nil
}

func packRoots(dir string) ([]string, error) {
	if _, err := os.Stat(filepath.Join(dir, manifestName)); err == nil {
		return []string{dir}, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var roots []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		root := filepath.Join(dir, entry.Name())
		if _, err := os.Stat(filepath.Join(root, manifestName)); err != nil {
			continue
		}
		roots = append(roots, root)
	}
	sort.Strings(roots)
	return roots, nil
}

func readPack(root string, opt ImportOptions) ([]*Pack, map[int64]string, error) {
	body, err := os.ReadFile(filepath.Join(root, manifestName))
	if err != nil {
		return nil, nil, err
	}
	body = bytes.TrimPrefix(body, []byte{0xEF, 0xBB, 0xBF})
	var man packManifest
	if err = json.Unmarshal(body, &man); err != nil {
		return nil, nil, fmt.Errorf("manifest: %w", err)
	}
	video, err := packType(man.Type)
	if err != nil {
		return nil, nil, err
	}
	if len(man.Stickers) == 0 {
		return nil, nil, fmt.Errorf("manifest has no stickers")
	}
	if len(man.Stickers) > maxStickersPerPack {
		return nil, nil, fmt.Errorf("manifest has %d stickers, limit is %d", len(man.Stickers), maxStickersPerPack)
	}
	if err = limitRunes("license", man.License, 64); err != nil {
		return nil, nil, err
	}
	if err = limitRunes("attribution", man.Attribution, 512); err != nil {
		return nil, nil, err
	}
	if err = limitRunes("source_url", man.SourceURL, 255); err != nil {
		return nil, nil, err
	}
	limit := maxStaticStickers
	if video {
		limit = maxAnimatedStickers
	}
	type built struct {
		item *Item
		path string
	}
	var builtItems []built
	seenFile := map[string]bool{}
	for i, st := range man.Stickers {
		path, err := safeJoin(root, st.File)
		if err != nil {
			return nil, nil, fmt.Errorf("sticker %d: %w", i, err)
		}
		if seenFile[path] {
			return nil, nil, fmt.Errorf("sticker %d: duplicate file %s", i, st.File)
		}
		seenFile[path] = true
		fileBody, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("sticker %s: %w", st.File, err)
		}
		if err = validateStickerFile(video, fileBody); err != nil {
			return nil, nil, fmt.Errorf("sticker %s: %w", st.File, err)
		}
		emo, err := cleanEmoji(st.Emoticon)
		if err != nil {
			return nil, nil, fmt.Errorf("sticker %s: %w", st.File, err)
		}
		en, langs, err := parseManifestKeywords(st.Keywords)
		if err != nil {
			return nil, nil, fmt.Errorf("sticker %s: %w", st.File, err)
		}
		item := &Item{
			Emoticon:     emo,
			MIME:         stickerMIMEFor(video),
			FileName:     filepath.Base(path),
			Width:        stickerSide,
			Height:       stickerSide,
			Video:        video,
			StorageClass: "stickers",
			Keywords:     en,
			LangKeywords: langs,
			Size:         int32(len(fileBody)),
		}
		builtItems = append(builtItems, built{item: item, path: path})
	}
	total := (len(builtItems) + limit - 1) / limit
	var packs []*Pack
	paths := map[int64]string{}
	for part := 0; part < total; part++ {
		index := part + 1
		short, err := splitShortName(man.ShortName, index, total)
		if err != nil {
			return nil, nil, err
		}
		title, err := splitPackTitle(man.Title, index, total)
		if err != nil {
			return nil, nil, err
		}
		end := (part + 1) * limit
		if end > len(builtItems) {
			end = len(builtItems)
		}
		chunk := builtItems[part*limit : end]
		items := make([]*Item, 0, len(chunk))
		for _, row := range chunk {
			item := row.item
			rel, err := filepath.Rel(root, row.path)
			if err != nil {
				return nil, nil, err
			}
			rel = filepath.ToSlash(rel)
			item.DocumentID = stableID("doc", "import", short, rel)
			item.AccessHash = stableID("access", "import", short, rel)
			item.ObjectPath = strconv.FormatInt(item.DocumentID, 10) + ".dat"
			if _, ok := paths[item.DocumentID]; ok {
				return nil, nil, fmt.Errorf("duplicate document for %s in %s", item.FileName, short)
			}
			paths[item.DocumentID] = row.path
			items = append(items, item)
		}
		packs = append(packs, &Pack{
			ID:               stableID("pack", short),
			AccessHash:       stableID("pack-access", short),
			ShortName:        short,
			Title:            title,
			Kind:             KindSticker,
			Official:         true,
			Videos:           video,
			Featured:         man.Featured || opt.Featured,
			InstalledDefault: man.InstalledDefault || opt.InstalledDefault,
			License:          man.License,
			Attribution:      man.Attribution,
			SourceURL:        man.SourceURL,
			Items:            items,
		})
	}
	return packs, paths, nil
}

func packType(raw string) (video bool, err error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "static", "webp":
		return false, nil
	case "video", "webm":
		return true, nil
	default:
		return false, fmt.Errorf("type %q must be static or video", raw)
	}
}

func stickerMIMEFor(video bool) string {
	if video {
		return "video/webm"
	}
	return "image/webp"
}

func splitShortName(base string, index, total int) (string, error) {
	base = strings.TrimSpace(base)
	if total <= 1 || index <= 1 {
		if !shortNameRE.MatchString(base) {
			return "", fmt.Errorf("short_name %q must match %s", base, shortNameRE)
		}
		return base, nil
	}
	suffix := strconv.Itoa(index)
	room := 32 - len(suffix)
	name := base
	if len(name) > room {
		name = name[:room]
	}
	out := name + suffix
	if !shortNameRE.MatchString(out) {
		return "", fmt.Errorf("short_name %q is not valid after splitting", out)
	}
	return out, nil
}

func splitPackTitle(title string, index, total int) (string, error) {
	title = strings.TrimSpace(title)
	if total <= 1 {
		return cleanTitle(title)
	}
	suffix := fmt.Sprintf(" %d/%d", index, total)
	max := maxStickerTitle - utf8.RuneCountInString(suffix)
	if max < 1 {
		max = 1
	}
	r := []rune(title)
	if len(r) > max {
		r = r[:max]
	}
	return cleanTitle(string(r) + suffix)
}

func limitRunes(field, value string, n int) error {
	if utf8.RuneCountInString(value) > n {
		return fmt.Errorf("%s is longer than %d characters", field, n)
	}
	return nil
}

func safeJoin(root, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || filepath.IsAbs(name) {
		return "", fmt.Errorf("file %q is not inside the pack directory", name)
	}
	full := filepath.Join(root, filepath.Clean(name))
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("file %q is not inside the pack directory", name)
	}
	return full, nil
}

func parseManifestKeywords(raw json.RawMessage) ([]string, map[string][]string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil, nil
	}
	switch raw[0] {
	case '[':
		var list []string
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, nil, fmt.Errorf("keywords: %w", err)
		}
		words, err := cleanKeywordList(list)
		return words, nil, err
	case '"':
		var one string
		if err := json.Unmarshal(raw, &one); err != nil {
			return nil, nil, fmt.Errorf("keywords: %w", err)
		}
		words, err := cleanKeywordList(splitKeywords(one))
		return words, nil, err
	case '{':
		var m map[string][]string
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, nil, fmt.Errorf("keywords: %w", err)
		}
		var en []string
		langs := map[string][]string{}
		keys := make([]string, 0, len(m))
		for key := range m {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			canon := canonicalEmojiLang(key)
			if canon == "" {
				return nil, nil, fmt.Errorf("keywords language %q is empty", key)
			}
			words, err := cleanKeywordList(m[key])
			if err != nil {
				return nil, nil, err
			}
			if canon == "en" {
				en = append(en, words...)
				continue
			}
			langs[canon] = append(langs[canon], words...)
		}
		if len(langs) == 0 {
			langs = nil
		}
		en, err := cleanKeywordList(en)
		return en, langs, err
	default:
		return nil, nil, fmt.Errorf("keywords must be a string, an array, or a language map")
	}
}

func cleanKeywordList(in []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, word := range in {
		word = strings.TrimSpace(word)
		if word == "" {
			continue
		}
		if utf8.RuneCountInString(word) > 64 {
			return nil, fmt.Errorf("keyword %q is longer than 64 characters", word)
		}
		key := foldText(word)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, word)
	}
	if len(out) > 32 {
		return nil, fmt.Errorf("a sticker has %d keywords, limit is 32", len(out))
	}
	return out, nil
}

// ImportPrepared validates dir and, unless DryRun, upserts the packs and
// uploads sticker bytes. Running it again updates the same rows and skips
// objects whose size already matches. A real run also upserts the built-in
// emoji packs, the same work as importstickers without -dir.
func ImportPrepared(dsn, dir string, opt ImportOptions) error {
	packs, paths, err := ReadPrepared(dir, opt)
	if err != nil {
		return err
	}
	logPrepared(packs, opt.DryRun)
	if opt.DryRun {
		return nil
	}
	if err = Import(dsn); err != nil {
		return err
	}
	s := Current()
	if s == nil {
		return fmt.Errorf("sticker catalog is not open")
	}
	mp, ok := s.mysql()
	if !ok {
		return fmt.Errorf("MySQL is not connected; set TEAMGRAM_MYSQL_DSN or -dsn")
	}
	s.mu.Lock()
	for _, p := range packs {
		if cur := s.byShort[strings.ToLower(p.ShortName)]; cur != nil && cur.ID != p.ID {
			s.mu.Unlock()
			return fmt.Errorf("short_name %s is already used by pack %d", p.ShortName, cur.ID)
		}
		if cur := s.byID[p.ID]; cur != nil && !strings.EqualFold(cur.ShortName, p.ShortName) {
			s.mu.Unlock()
			return fmt.Errorf("pack id %d is already short_name %s", p.ID, cur.ShortName)
		}
		for _, item := range p.Items {
			if link := s.docs[item.DocumentID]; link != nil && link.pack != nil && link.pack.ID != p.ID {
				s.mu.Unlock()
				return fmt.Errorf("document %d is already in %s", item.DocumentID, link.pack.ShortName)
			}
		}
	}
	s.mu.Unlock()
	storage, err := objectstore.Open()
	if err != nil {
		return err
	}
	if storage == nil {
		return fmt.Errorf("object storage is not configured (TELLOMI_S3_*)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if err = mp.savePrepared(ctx, packs); err != nil {
		return err
	}
	if err = uploadPrepared(ctx, storage, packs, paths); err != nil {
		return err
	}
	log.Printf("importstickers: wrote %d packs", len(packs))
	return nil
}

func logPrepared(packs []*Pack, dry bool) {
	var stickers int
	for _, p := range packs {
		kind := "static"
		if p.Videos {
			kind = "video"
		}
		log.Printf("importstickers: %s short=%s %s stickers=%d featured=%t installed_default=%t",
			p.Title, p.ShortName, kind, len(p.Items), p.Featured, p.InstalledDefault)
		stickers += len(p.Items)
	}
	mode := "import"
	if dry {
		mode = "dry-run"
	}
	log.Printf("importstickers: %s %d packs, %d stickers", mode, len(packs), stickers)
}

func uploadPrepared(ctx context.Context, client *objectstore.Client, packs []*Pack, paths map[int64]string) error {
	if client == nil {
		return fmt.Errorf("object storage is not configured (TELLOMI_S3_*)")
	}
	var uploaded, skipped int
	for _, p := range packs {
		for _, item := range p.Items {
			path := paths[item.DocumentID]
			if path == "" {
				return fmt.Errorf("no file for document %d", item.DocumentID)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read %s: %w", path, err)
			}
			size, err := client.Size(ctx, item.StorageClass, item.ObjectPath)
			if err == nil && size == int64(len(body)) {
				skipped++
				continue
			}
			if err != nil && !errors.Is(err, objectstore.ErrNotFound) {
				return fmt.Errorf("stat %s: %w", item.ObjectPath, err)
			}
			if err = client.Put(ctx, item.StorageClass, item.ObjectPath, item.MIME, body); err != nil {
				return fmt.Errorf("upload %s: %w", path, err)
			}
			uploaded++
		}
	}
	log.Printf("importstickers: uploaded %d objects, skipped %d already stored", uploaded, skipped)
	return nil
}
