package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	// reloadInterval is how often a running bff re-reads imported packs.
	// Official clients refetch when the list hash changes; 45s is inside
	// the 30–60s window and does not restart the process.
	reloadInterval = 45 * time.Second
	// recentSessions is how far back a catalog broadcast looks. Clients
	// that have been offline longer still pick up the new hash the next
	// time they call getAllStickers.
	recentSessions = 7 * 24 * time.Hour
	recentUserCap  = 2000

	catalogStickers = "stickers"
	catalogGifs     = "gifs"
)

const catalogVersionDDL = `
CREATE TABLE IF NOT EXISTS tellomi_catalog_version (
  name VARCHAR(32) NOT NULL,
  version BIGINT NOT NULL,
  PRIMARY KEY (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

// reloadDiff is what a catalog reload changed in memory.
type reloadDiff struct {
	added   int
	updated int
	removed int
	kinds   kindFlags
}

func (d reloadDiff) any() bool { return d.added+d.updated+d.removed > 0 }

func (d *reloadDiff) note(p *Pack) {
	regular, masks, emojis := listKind(p)
	if regular {
		d.kinds.regular = true
	}
	if masks {
		d.kinds.masks = true
	}
	if emojis {
		d.kinds.emojis = true
	}
}

// kindFlags is which installed list an updateStickerSets should refresh.
// A regular pack sets none of the TL flags. One update refreshes one list,
// matching the official constructors.
type kindFlags struct {
	regular bool
	masks   bool
	emojis  bool
}

func (k kindFlags) any() bool { return k.regular || k.masks || k.emojis }

func (k *kindFlags) add(o kindFlags) {
	k.regular = k.regular || o.regular
	k.masks = k.masks || o.masks
	k.emojis = k.emojis || o.emojis
}

// listKind reports which get*Stickers list shows the pack. System sets are
// fetched by constructor and are not part of those lists.
func listKind(p *Pack) (regular, masks, emojis bool) {
	if p == nil || p.system() {
		return false, false, false
	}
	if p.Emojis {
		return false, false, true
	}
	if p.Masks {
		return false, true, false
	}
	return true, false, false
}

func (s *Service) startReloadLoop() {
	if _, ok := s.mysql(); !ok {
		return
	}
	go s.reloadLoop()
}

func (s *Service) reloadLoop() {
	ticker := time.NewTicker(reloadInterval)
	defer ticker.Stop()
	for range ticker.C {
		s.reloadTick(context.Background())
	}
}

func (s *Service) reloadTick(ctx context.Context) {
	diff, err := s.ReloadImported(ctx)
	if err != nil {
		logx.Errorf("stickers reload: %v", err)
	} else if diff.any() {
		logx.Infof("stickers reload: added %d, updated %d, removed %d", diff.added, diff.updated, diff.removed)
		s.noteCatalogChange(diff.kinds)
	}
	if err = s.ReloadGifs(ctx); err != nil {
		logx.Errorf("gif reload: %v", err)
	}
}

// ReloadImported re-reads tellomi_sticker_packs. New ids are added. A pack
// whose title, flags, or documents changed is removed and added again.
// Readers take s.mu, and the swap does too, so getStickerSet never observes
// a half-updated map. A matching catalog version skips the read.
func (s *Service) ReloadImported(ctx context.Context) (reloadDiff, error) {
	mp, ok := s.mysql()
	if !ok {
		return reloadDiff{}, nil
	}
	v1, err := catalogVersion(ctx, mp.db, catalogStickers)
	if err != nil {
		return reloadDiff{}, err
	}
	s.mu.Lock()
	same := s.stickerVersionLoaded && s.stickerVersion == v1
	s.mu.Unlock()
	if same {
		return reloadDiff{}, nil
	}
	loaded, v2, stable, err := loadStablePacks(ctx, mp.db)
	if err != nil {
		return reloadDiff{}, err
	}
	diff := s.applyImported(loaded, stable)
	if stable {
		s.mu.Lock()
		s.stickerVersion = v2
		s.stickerVersionLoaded = true
		s.mu.Unlock()
	}
	return diff, nil
}

func loadStablePacks(ctx context.Context, db *sql.DB) ([]*Pack, int64, bool, error) {
	loaded, err := readImportedPacks(ctx, db)
	if err != nil {
		return nil, 0, false, err
	}
	v, err := catalogVersion(ctx, db, catalogStickers)
	if err != nil {
		return nil, 0, false, err
	}
	again, err := readImportedPacks(ctx, db)
	if err != nil {
		return nil, 0, false, err
	}
	v2, err := catalogVersion(ctx, db, catalogStickers)
	if err != nil {
		return nil, 0, false, err
	}
	if v == v2 {
		return again, v2, true, nil
	}
	return loaded, v, false, nil
}

func readImportedPacks(ctx context.Context, db *sql.DB) ([]*Pack, error) {
	rows, err := db.QueryContext(ctx, `
SELECT id, access_hash, short_name, title, kind, official, masks, emojis, animated, videos, text_color, featured, installed_default, license, attribution, source_url
FROM tellomi_sticker_packs ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Pack
	for rows.Next() {
		p := &Pack{}
		var official, masks, emojis, animated, videos, textColor, featured, installed int
		if err = rows.Scan(&p.ID, &p.AccessHash, &p.ShortName, &p.Title, &p.Kind, &official, &masks, &emojis, &animated, &videos, &textColor, &featured, &installed, &p.License, &p.Attribution, &p.SourceURL); err != nil {
			return nil, err
		}
		p.Official = official != 0
		p.Masks = masks != 0
		p.Emojis = emojis != 0
		p.Animated = animated != 0
		p.Videos = videos != 0
		p.TextColor = textColor != 0
		p.Featured = featured != 0
		p.InstalledDefault = installed != 0
		items, err := loadPackItems(ctx, db, p.ID)
		if err != nil {
			return nil, err
		}
		p.Items = items
		out = append(out, p)
	}
	return out, rows.Err()
}

// applyImported swaps new and changed packs under s.mu. dropMissing removes
// packs that this process previously loaded from MySQL and that the snapshot
// no longer contains. An empty snapshot does not wipe memory.
func (s *Service) applyImported(loaded []*Pack, dropMissing bool) reloadDiff {
	s.mu.Lock()
	defer s.mu.Unlock()
	var diff reloadDiff
	seen := map[int64]bool{}
	for _, src := range loaded {
		if src == nil || src.ID == 0 {
			continue
		}
		seen[src.ID] = true
		next := clonePack(src)
		old := s.byID[next.ID]
		if old != nil {
			preserveRuntime(old, next)
			if packFingerprint(old) == packFingerprint(next) {
				continue
			}
			s.removePack(old)
		}
		key := strings.ToLower(next.ShortName)
		if cur := s.byShort[key]; cur != nil && cur.ID != next.ID {
			logx.Errorf("stickers skip imported pack %s: short name is taken", next.ShortName)
			if old != nil {
				s.addPack(old)
			}
			continue
		}
		s.addPack(next)
		if old == nil {
			diff.added++
		} else {
			diff.updated++
		}
		diff.note(next)
	}
	if s.catalogIDs == nil {
		s.catalogIDs = map[int64]bool{}
	}
	if dropMissing && len(loaded) > 0 {
		for id := range s.catalogIDs {
			if seen[id] {
				continue
			}
			if p := s.byID[id]; p != nil && !p.system() {
				s.removePack(p)
				diff.removed++
				diff.note(p)
			}
			delete(s.catalogIDs, id)
		}
	}
	for id := range seen {
		s.catalogIDs[id] = true
	}
	return diff
}

func preserveRuntime(old, next *Pack) {
	if old == nil || next == nil {
		return
	}
	next.Owner = old.Owner
	if next.ThumbID == 0 {
		next.ThumbID = old.ThumbID
	}
	prev := map[int64]*Item{}
	for _, item := range old.Items {
		if item != nil {
			prev[item.DocumentID] = item
		}
	}
	for _, item := range next.Items {
		if item == nil || item.MaskCoords != nil {
			continue
		}
		if was := prev[item.DocumentID]; was != nil {
			item.MaskCoords = was.MaskCoords
		}
	}
}

func clonePack(p *Pack) *Pack {
	if p == nil {
		return nil
	}
	cp := *p
	cp.Items = make([]*Item, len(p.Items))
	for i, item := range p.Items {
		cp.Items[i] = cloneItem(item)
	}
	return &cp
}

func packFingerprint(p *Pack) string {
	if p == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d|%d|%s|%s|%s|%t|%t|%t|%t|%t|%t|%t|%t|%s|%s|%s|%d|%d",
		p.ID, p.AccessHash, p.ShortName, p.Title, p.Kind,
		p.Official, p.Masks, p.Emojis, p.Animated, p.Videos, p.TextColor, p.Featured, p.InstalledDefault,
		p.License, p.Attribution, p.SourceURL, p.Owner, p.ThumbID)
	for _, item := range p.Items {
		if item == nil {
			continue
		}
		fmt.Fprintf(&b, "\n%d|%d|%s|%s|%d|%d|%d|%t|%t|%t|%s|%s|%s|%s|%s",
			item.DocumentID, item.AccessHash, item.Emoticon, item.MIME, item.Size, item.Width, item.Height,
			item.Animated, item.Video, item.CustomEmoji, item.FileName, item.StorageClass, item.ObjectPath, item.AssetPath,
			encodeKeywords(item))
	}
	return b.String()
}

func catalogVersion(ctx context.Context, db *sql.DB, name string) (int64, error) {
	var v int64
	err := db.QueryRowContext(ctx, `SELECT version FROM tellomi_catalog_version WHERE name=?`, name).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "tellomi_catalog_version") {
		return 0, nil
	}
	return v, err
}

func bumpCatalogVersion(ctx context.Context, db *sql.DB, name string) error {
	if db == nil || name == "" {
		return nil
	}
	err := bumpCatalogOnce(ctx, db, name)
	if err == nil {
		return nil
	}
	if _, cerr := db.ExecContext(ctx, catalogVersionDDL); cerr != nil {
		return err
	}
	return bumpCatalogOnce(ctx, db, name)
}

func bumpCatalogOnce(ctx context.Context, db *sql.DB, name string) error {
	_, err := db.ExecContext(ctx, `
INSERT INTO tellomi_catalog_version (name, version) VALUES (?, 1)
ON DUPLICATE KEY UPDATE version = version + 1`, name)
	return err
}
