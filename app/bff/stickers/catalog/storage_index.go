package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/zeromicro/go-zero/core/logx"
)

// objectLoc is one public sticker or emoji object.
type objectLoc struct {
	Class string
	Path  string
}

// DocumentStorage is the object class and key for a catalog document.
// class is "stickers" or "emoji" when the bytes live in the public media
// bucket. public is false for a pack a user created. ok is false when the
// id is not in the catalog.
func (s *Service) DocumentStorage(id int64) (class, path string, public, ok bool) {
	if s == nil || id == 0 {
		return "", "", false, false
	}
	_, public, ok = s.DocumentAccess(id)
	if !ok {
		return "", "", false, false
	}
	s.mu.Lock()
	link := s.docs[id]
	s.mu.Unlock()
	if link == nil || link.item == nil {
		return "", "", public, false
	}
	class = link.item.StorageClass
	path = link.item.ObjectPath
	if path == "" {
		path = fmt.Sprintf("%d.dat", id)
	}
	if class != "stickers" && class != "emoji" {
		return "", path, public, false
	}
	return class, path, public, true
}

// PublicObject is the stickers or emoji object for an official catalog
// document. The process catalog wins. DFS does not host that catalog, so
// it also loads tellomi_sticker_items and falls back to the built-in packs.
func PublicObject(id int64) (class, path string, ok bool) {
	if id == 0 {
		return "", "", false
	}
	if s := Current(); s != nil {
		class, path, public, found := s.DocumentStorage(id)
		if found && public {
			return class, path, true
		}
		if _, _, known := s.DocumentAccess(id); known {
			return "", "", false
		}
	}
	if loc, ok := builtinObject(id); ok {
		return loc.Class, loc.Path, true
	}
	if idx := publicIndex.Load(); idx != nil {
		if loc, ok := idx.(map[int64]objectLoc)[id]; ok {
			return loc.Class, loc.Path, true
		}
	}
	return "", "", false
}

var (
	builtinOnce sync.Once
	builtinIdx  map[int64]objectLoc
	publicIndex atomic.Value // map[int64]objectLoc
)

func builtinObject(id int64) (objectLoc, bool) {
	builtinOnce.Do(func() {
		builtinIdx = map[int64]objectLoc{}
		for _, p := range builtinPacks() {
			addPublicPack(builtinIdx, p)
		}
	})
	loc, ok := builtinIdx[id]
	return loc, ok
}

func addPublicPack(idx map[int64]objectLoc, p *Pack) {
	if p == nil || p.Owner != 0 {
		return
	}
	for _, item := range p.Items {
		if item == nil || item.DocumentID == 0 {
			continue
		}
		if item.StorageClass != "stickers" && item.StorageClass != "emoji" {
			continue
		}
		if _, exists := idx[item.DocumentID]; exists {
			continue
		}
		path := item.ObjectPath
		if path == "" {
			path = fmt.Sprintf("%d.dat", item.DocumentID)
		}
		idx[item.DocumentID] = objectLoc{Class: item.StorageClass, Path: path}
	}
}

// indexPublicRows keeps the first stickers or emoji location for each id.
// A user-owned row (owner != 0) is skipped. Later rows do not replace it.
func indexPublicRows(rows []publicRow) map[int64]objectLoc {
	idx := make(map[int64]objectLoc, len(rows))
	for _, row := range rows {
		if row.Owner != 0 || row.DocumentID == 0 {
			continue
		}
		if row.Class != "stickers" && row.Class != "emoji" {
			continue
		}
		if _, exists := idx[row.DocumentID]; exists {
			continue
		}
		path := row.Path
		if path == "" {
			path = fmt.Sprintf("%d.dat", row.DocumentID)
		}
		idx[row.DocumentID] = objectLoc{Class: row.Class, Path: path}
	}
	return idx
}

type publicRow struct {
	DocumentID int64
	Class      string
	Path       string
	Owner      int64
}

const publicObjectSQL = `
SELECT i.document_id, i.storage_class, i.object_path, IFNULL(o.user_id, 0)
FROM tellomi_sticker_items i
LEFT JOIN tellomi_sticker_owners o ON o.pack_id = i.pack_id
ORDER BY i.document_id, i.pack_id`

// ReloadPublicObjects reads official sticker and emoji keys from MySQL.
// It does not insert packs or upload objects. A missing table is an error
// so the caller can retry after the bff has created the schema.
func ReloadPublicObjects(ctx context.Context, dsn string) error {
	if dsn == "" {
		return nil
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, publicObjectSQL)
	if err != nil {
		return err
	}
	defer rows.Close()
	var list []publicRow
	for rows.Next() {
		var row publicRow
		if err = rows.Scan(&row.DocumentID, &row.Class, &row.Path, &row.Owner); err != nil {
			return err
		}
		list = append(list, row)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	publicIndex.Store(indexPublicRows(list))
	return nil
}

// WatchPublicObjects refreshes the DFS sticker index. The first load is
// synchronous, up to 5s, so a download that arrives at startup is routed.
func WatchPublicObjects(dsn string) {
	if dsn == "" {
		return
	}
	load := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := ReloadPublicObjects(ctx, dsn); err != nil {
			logx.Errorf("catalog public objects: %v", err)
		}
	}
	load()
	go func() {
		tick := time.NewTicker(reloadInterval)
		defer tick.Stop()
		for range tick.C {
			load()
		}
	}()
}
