package catalog

import (
	"context"
	"database/sql"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/zeromicro/go-zero/core/logx"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS tellomi_sticker_packs (
  id BIGINT NOT NULL,
  access_hash BIGINT NOT NULL,
  short_name VARCHAR(64) NOT NULL,
  title VARCHAR(128) NOT NULL,
  kind VARCHAR(64) NOT NULL,
  official TINYINT NOT NULL DEFAULT 1,
  masks TINYINT NOT NULL DEFAULT 0,
  emojis TINYINT NOT NULL DEFAULT 0,
  animated TINYINT NOT NULL DEFAULT 0,
  videos TINYINT NOT NULL DEFAULT 0,
  text_color TINYINT NOT NULL DEFAULT 0,
  featured TINYINT NOT NULL DEFAULT 0,
  installed_default TINYINT NOT NULL DEFAULT 0,
  license VARCHAR(64) NOT NULL,
  attribution VARCHAR(512) NOT NULL,
  source_url VARCHAR(255) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY tellomi_sticker_packs_short (short_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS tellomi_sticker_items (
  pack_id BIGINT NOT NULL,
  document_id BIGINT NOT NULL,
  access_hash BIGINT NOT NULL,
  emoticon VARCHAR(32) NOT NULL,
  sort_order INT NOT NULL,
  mime_type VARCHAR(64) NOT NULL,
  file_size INT NOT NULL,
  width INT NOT NULL,
  height INT NOT NULL,
  animated TINYINT NOT NULL DEFAULT 0,
  video TINYINT NOT NULL DEFAULT 0,
  custom_emoji TINYINT NOT NULL DEFAULT 0,
  file_name VARCHAR(128) NOT NULL,
  storage_class VARCHAR(16) NOT NULL,
  object_path VARCHAR(255) NOT NULL,
  asset_path VARCHAR(255) NOT NULL,
  keywords VARCHAR(512) NOT NULL DEFAULT '',
  PRIMARY KEY (pack_id, document_id),
  KEY tellomi_sticker_items_doc (document_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS tellomi_sticker_installs (
  user_id BIGINT NOT NULL,
  pack_id BIGINT NOT NULL,
  installed TINYINT NOT NULL,
  archived TINYINT NOT NULL DEFAULT 0,
  sort_order INT NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, pack_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS tellomi_sticker_recent (
  user_id BIGINT NOT NULL,
  document_id BIGINT NOT NULL,
  attached TINYINT NOT NULL DEFAULT 0,
  date_ts INT NOT NULL,
  PRIMARY KEY (user_id, document_id, attached)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS tellomi_sticker_faved (
  user_id BIGINT NOT NULL,
  document_id BIGINT NOT NULL,
  date_ts INT NOT NULL,
  PRIMARY KEY (user_id, document_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS tellomi_reaction_counts (
  emoticon VARCHAR(32) NOT NULL,
  use_count BIGINT NOT NULL,
  PRIMARY KEY (emoticon)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS tellomi_reaction_recent (
  user_id BIGINT NOT NULL,
  emoticon VARCHAR(32) NOT NULL,
  date_ts INT NOT NULL,
  PRIMARY KEY (user_id, emoticon),
  KEY tellomi_reaction_recent_user (user_id, date_ts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS tellomi_sticker_owners (
  pack_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  thumb_document_id BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (pack_id),
  KEY tellomi_sticker_owners_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS tellomi_featured_reads (
  user_id BIGINT NOT NULL,
  pack_id BIGINT NOT NULL,
  PRIMARY KEY (user_id, pack_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS tellomi_saved_gifs (
  user_id BIGINT NOT NULL,
  document_id BIGINT NOT NULL,
  access_hash BIGINT NOT NULL,
  date_ts INT NOT NULL,
  mime_type VARCHAR(64) NOT NULL,
  file_size INT NOT NULL,
  width INT NOT NULL,
  height INT NOT NULL,
  file_name VARCHAR(128) NOT NULL,
  video TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, document_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
`

type mysqlPersist struct {
	db *sql.DB
}

func (s *Service) attachMySQL(dsn string) error {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return err
	}
	if err = execSchema(ctx, db); err != nil {
		_ = db.Close()
		return err
	}
	if err = upsertPacks(ctx, db, s.packs); err != nil {
		_ = db.Close()
		return err
	}
	if err = s.loadUserState(ctx, db); err != nil {
		_ = db.Close()
		return err
	}
	if err = s.loadReactionState(ctx, db); err != nil {
		_ = db.Close()
		return err
	}
	s.persist = mysqlPersist{db: db}
	if err = s.loadManageState(ctx, db); err != nil {
		_ = db.Close()
		s.persist = nil
		return err
	}
	return nil
}

func execSchema(ctx context.Context, db *sql.DB) error {
	for _, stmt := range strings.Split(schemaSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func upsertPacks(ctx context.Context, db *sql.DB, packs []*Pack) error {
	for _, p := range packs {
		_, err := db.ExecContext(ctx, `
INSERT INTO tellomi_sticker_packs
  (id, access_hash, short_name, title, kind, official, masks, emojis, animated, videos, text_color, featured, installed_default, license, attribution, source_url)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  access_hash=VALUES(access_hash), title=VALUES(title), kind=VALUES(kind),
  official=VALUES(official), masks=VALUES(masks), emojis=VALUES(emojis),
  animated=VALUES(animated), videos=VALUES(videos), text_color=VALUES(text_color),
  featured=VALUES(featured), installed_default=VALUES(installed_default),
  license=VALUES(license), attribution=VALUES(attribution), source_url=VALUES(source_url)`,
			p.ID, p.AccessHash, p.ShortName, p.Title, p.Kind, b(p.Official), b(p.Masks), b(p.Emojis),
			b(p.Animated), b(p.Videos), b(p.TextColor), b(p.Featured), b(p.InstalledDefault),
			p.License, p.Attribution, p.SourceURL)
		if err != nil {
			return err
		}
		for i, item := range p.Items {
			_, err = db.ExecContext(ctx, `
INSERT INTO tellomi_sticker_items
  (pack_id, document_id, access_hash, emoticon, sort_order, mime_type, file_size, width, height, animated, video, custom_emoji, file_name, storage_class, object_path, asset_path, keywords)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  access_hash=VALUES(access_hash), emoticon=VALUES(emoticon), sort_order=VALUES(sort_order),
  mime_type=VALUES(mime_type), file_size=VALUES(file_size), width=VALUES(width), height=VALUES(height),
  animated=VALUES(animated), video=VALUES(video), custom_emoji=VALUES(custom_emoji),
  file_name=VALUES(file_name), storage_class=VALUES(storage_class), object_path=VALUES(object_path),
  asset_path=VALUES(asset_path), keywords=VALUES(keywords)`,
				p.ID, item.DocumentID, item.AccessHash, item.Emoticon, i, item.MIME, item.Size, item.Width, item.Height,
				b(item.Animated), b(item.Video), b(item.CustomEmoji), item.FileName, item.StorageClass, item.ObjectPath,
				item.AssetPath, strings.Join(item.Keywords, "\n"))
			if err != nil {
				return err
			}
		}
		if _, err = db.ExecContext(ctx, `
INSERT INTO sticker_sets (short_name, title, count, emoji, thumb_path, public, deleted)
VALUES (?, ?, ?, ?, ?, 1, 0)
ON DUPLICATE KEY UPDATE title=VALUES(title), count=VALUES(count), thumb_path=VALUES(thumb_path), public=1, deleted=0`,
			p.ShortName, p.Title, len(p.Items), b(p.Emojis), thumbPath(p)); err != nil {
			logx.Errorf("stickers link preview %s: %v", p.ShortName, err)
		}
	}
	return nil
}

func thumbPath(p *Pack) string {
	if len(p.Items) == 0 {
		return ""
	}
	return p.Items[0].StorageClass + "/" + p.Items[0].ObjectPath
}

func (s *Service) loadUserState(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT user_id, pack_id, installed, archived, sort_order FROM tellomi_sticker_installs`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var userID, packID int64
		var installed, archived, order int
		if err = rows.Scan(&userID, &packID, &installed, &archived, &order); err != nil {
			return err
		}
		s.state(userID).install[packID] = installRow{Installed: installed != 0, Archived: archived != 0, Order: int32(order)}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	recent, err := db.QueryContext(ctx, `SELECT user_id, document_id, attached, date_ts FROM tellomi_sticker_recent ORDER BY date_ts DESC`)
	if err != nil {
		return err
	}
	defer recent.Close()
	for recent.Next() {
		var userID, docID int64
		var attached, date int
		if err = recent.Scan(&userID, &docID, &attached, &date); err != nil {
			return err
		}
		s.state(userID).recent = append(s.state(userID).recent, recentRow{DocumentID: docID, Attached: attached != 0, Date: int32(date)})
	}
	faved, err := db.QueryContext(ctx, `SELECT user_id, document_id, date_ts FROM tellomi_sticker_faved ORDER BY date_ts DESC`)
	if err != nil {
		return err
	}
	defer faved.Close()
	for faved.Next() {
		var userID, docID int64
		var date int
		if err = faved.Scan(&userID, &docID, &date); err != nil {
			return err
		}
		s.state(userID).faved = append(s.state(userID).faved, favedRow{DocumentID: docID, Date: int32(date)})
	}
	return faved.Err()
}

func (m mysqlPersist) saveInstall(userID, packID int64, row installRow) error {
	_, err := m.db.Exec(`
INSERT INTO tellomi_sticker_installs (user_id, pack_id, installed, archived, sort_order)
VALUES (?,?,?,?,?)
ON DUPLICATE KEY UPDATE installed=VALUES(installed), archived=VALUES(archived), sort_order=VALUES(sort_order)`,
		userID, packID, b(row.Installed), b(row.Archived), row.Order)
	return err
}

func (m mysqlPersist) saveRecent(userID int64, row recentRow, remove bool) error {
	if remove {
		_, err := m.db.Exec(`DELETE FROM tellomi_sticker_recent WHERE user_id=? AND document_id=? AND attached=?`, userID, row.DocumentID, b(row.Attached))
		return err
	}
	_, err := m.db.Exec(`
INSERT INTO tellomi_sticker_recent (user_id, document_id, attached, date_ts)
VALUES (?,?,?,?)
ON DUPLICATE KEY UPDATE date_ts=VALUES(date_ts)`, userID, row.DocumentID, b(row.Attached), row.Date)
	return err
}

func (m mysqlPersist) saveFaved(userID, documentID int64, date int32, remove bool) error {
	if remove {
		_, err := m.db.Exec(`DELETE FROM tellomi_sticker_faved WHERE user_id=? AND document_id=?`, userID, documentID)
		return err
	}
	_, err := m.db.Exec(`
INSERT INTO tellomi_sticker_faved (user_id, document_id, date_ts) VALUES (?,?,?)
ON DUPLICATE KEY UPDATE date_ts=VALUES(date_ts)`, userID, documentID, date)
	return err
}

func (s *Service) loadReactionState(ctx context.Context, db *sql.DB) error {
	if s.reactionUses == nil {
		s.reactionUses = map[string]int64{}
	}
	counts, err := db.QueryContext(ctx, `SELECT emoticon, use_count FROM tellomi_reaction_counts`)
	if err != nil {
		return err
	}
	defer counts.Close()
	for counts.Next() {
		var emo string
		var n int64
		if err = counts.Scan(&emo, &n); err != nil {
			return err
		}
		s.reactionUses[emo] = n
	}
	if err = counts.Err(); err != nil {
		return err
	}
	recent, err := db.QueryContext(ctx, `SELECT user_id, emoticon, date_ts FROM tellomi_reaction_recent ORDER BY date_ts DESC`)
	if err != nil {
		return err
	}
	defer recent.Close()
	for recent.Next() {
		var userID int64
		var emo string
		var date int
		if err = recent.Scan(&userID, &emo, &date); err != nil {
			return err
		}
		st := s.state(userID)
		if len(st.reactionRecent) >= maxRecentReactions {
			continue
		}
		st.reactionRecent = append(st.reactionRecent, emo)
	}
	return recent.Err()
}

func (m mysqlPersist) saveReactionUse(emoticon string, count int64) error {
	_, err := m.db.Exec(`
INSERT INTO tellomi_reaction_counts (emoticon, use_count) VALUES (?, ?)
ON DUPLICATE KEY UPDATE use_count=VALUES(use_count)`, emoticon, count)
	return err
}

func (m mysqlPersist) saveReactionRecent(userID int64, emoticon string, date int32) error {
	_, err := m.db.Exec(`
INSERT INTO tellomi_reaction_recent (user_id, emoticon, date_ts) VALUES (?, ?, ?)
ON DUPLICATE KEY UPDATE date_ts=VALUES(date_ts)`, userID, emoticon, date)
	return err
}

func (m mysqlPersist) clearReactionRecent(userID int64) error {
	_, err := m.db.Exec(`DELETE FROM tellomi_reaction_recent WHERE user_id=?`, userID)
	return err
}

func b(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (s *Service) loadManageState(ctx context.Context, db *sql.DB) error {
	if err := s.loadOwnedPacks(ctx, db); err != nil {
		return err
	}
	reads, err := db.QueryContext(ctx, `SELECT user_id, pack_id FROM tellomi_featured_reads`)
	if err != nil {
		return err
	}
	defer reads.Close()
	for reads.Next() {
		var userID, packID int64
		if err = reads.Scan(&userID, &packID); err != nil {
			return err
		}
		st := s.state(userID)
		if st.featuredRead == nil {
			st.featuredRead = map[int64]bool{}
		}
		st.featuredRead[packID] = true
	}
	if err = reads.Err(); err != nil {
		return err
	}
	gifs, err := db.QueryContext(ctx, `SELECT user_id, document_id, access_hash, date_ts, mime_type, file_size, width, height, file_name, video FROM tellomi_saved_gifs ORDER BY date_ts DESC`)
	if err != nil {
		return err
	}
	defer gifs.Close()
	for gifs.Next() {
		var userID int64
		var row gifRow
		var video int
		if err = gifs.Scan(&userID, &row.DocumentID, &row.AccessHash, &row.Date, &row.MIME, &row.Size, &row.Width, &row.Height, &row.FileName, &video); err != nil {
			return err
		}
		row.Video = video != 0
		st := s.state(userID)
		if len(st.gifs) >= maxSavedGifs {
			continue
		}
		st.gifs = append(st.gifs, row)
	}
	return gifs.Err()
}

func (s *Service) loadOwnedPacks(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
SELECT p.id, p.access_hash, p.short_name, p.title, p.kind, p.official, p.masks, p.emojis, p.animated, p.videos, p.text_color, p.featured, p.installed_default, p.license, p.attribution, p.source_url, o.user_id, o.thumb_document_id
FROM tellomi_sticker_owners o
JOIN tellomi_sticker_packs p ON p.id = o.pack_id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var p Pack
		var official, masks, emojis, animated, videos, textColor, featured, installed int
		if err = rows.Scan(&p.ID, &p.AccessHash, &p.ShortName, &p.Title, &p.Kind, &official, &masks, &emojis, &animated, &videos, &textColor, &featured, &installed, &p.License, &p.Attribution, &p.SourceURL, &p.Owner, &p.ThumbID); err != nil {
			return err
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
			return err
		}
		p.Items = items
		if cur := s.byID[p.ID]; cur != nil {
			cur.Owner = p.Owner
			cur.ThumbID = p.ThumbID
			continue
		}
		key := strings.ToLower(p.ShortName)
		if _, taken := s.byShort[key]; taken {
			logx.Errorf("stickers skip owned pack %s: short name is taken", p.ShortName)
			continue
		}
		owned := p
		s.addPack(&owned)
	}
	return rows.Err()
}

func loadPackItems(ctx context.Context, db *sql.DB, packID int64) ([]*Item, error) {
	rows, err := db.QueryContext(ctx, `
SELECT document_id, access_hash, emoticon, mime_type, file_size, width, height, animated, video, custom_emoji, file_name, storage_class, object_path, asset_path, keywords
FROM tellomi_sticker_items WHERE pack_id=? ORDER BY sort_order`, packID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []*Item
	for rows.Next() {
		item := &Item{}
		var animated, video, custom int
		var keywords string
		if err = rows.Scan(&item.DocumentID, &item.AccessHash, &item.Emoticon, &item.MIME, &item.Size, &item.Width, &item.Height, &animated, &video, &custom, &item.FileName, &item.StorageClass, &item.ObjectPath, &item.AssetPath, &keywords); err != nil {
			return nil, err
		}
		item.Animated = animated != 0
		item.Video = video != 0
		item.CustomEmoji = custom != 0
		if keywords != "" {
			item.Keywords = strings.Split(keywords, "\n")
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (m mysqlPersist) saveOwnedPack(p *Pack) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := upsertPacks(ctx, m.db, []*Pack{p}); err != nil {
		return err
	}
	if _, err := m.db.ExecContext(ctx, `
INSERT INTO tellomi_sticker_owners (pack_id, user_id, thumb_document_id) VALUES (?,?,?)
ON DUPLICATE KEY UPDATE user_id=VALUES(user_id), thumb_document_id=VALUES(thumb_document_id)`,
		p.ID, p.Owner, p.ThumbID); err != nil {
		return err
	}
	if _, err := m.db.ExecContext(ctx, `DELETE FROM tellomi_sticker_items WHERE pack_id=?`, p.ID); err != nil {
		return err
	}
	for i, item := range p.Items {
		if _, err := m.db.ExecContext(ctx, `
INSERT INTO tellomi_sticker_items
  (pack_id, document_id, access_hash, emoticon, sort_order, mime_type, file_size, width, height, animated, video, custom_emoji, file_name, storage_class, object_path, asset_path, keywords)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			p.ID, item.DocumentID, item.AccessHash, item.Emoticon, i, item.MIME, item.Size, item.Width, item.Height,
			b(item.Animated), b(item.Video), b(item.CustomEmoji), item.FileName, item.StorageClass, item.ObjectPath,
			item.AssetPath, strings.Join(item.Keywords, "\n")); err != nil {
			return err
		}
	}
	return nil
}

func (m mysqlPersist) deleteOwnedPack(packID int64) error {
	for _, stmt := range []string{
		`DELETE FROM tellomi_sticker_items WHERE pack_id=?`,
		`DELETE FROM tellomi_sticker_owners WHERE pack_id=?`,
		`DELETE FROM tellomi_sticker_installs WHERE pack_id=?`,
		`DELETE FROM tellomi_featured_reads WHERE pack_id=?`,
		`DELETE FROM tellomi_sticker_packs WHERE id=?`,
	} {
		if _, err := m.db.Exec(stmt, packID); err != nil {
			return err
		}
	}
	return nil
}

func (m mysqlPersist) saveFeaturedRead(userID, packID int64) error {
	_, err := m.db.Exec(`INSERT IGNORE INTO tellomi_featured_reads (user_id, pack_id) VALUES (?,?)`, userID, packID)
	return err
}

func (m mysqlPersist) saveGif(userID int64, row gifRow) error {
	_, err := m.db.Exec(`
INSERT INTO tellomi_saved_gifs (user_id, document_id, access_hash, date_ts, mime_type, file_size, width, height, file_name, video)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE access_hash=VALUES(access_hash), date_ts=VALUES(date_ts), mime_type=VALUES(mime_type), file_size=VALUES(file_size), width=VALUES(width), height=VALUES(height), file_name=VALUES(file_name), video=VALUES(video)`,
		userID, row.DocumentID, row.AccessHash, row.Date, row.MIME, row.Size, row.Width, row.Height, row.FileName, b(row.Video))
	return err
}

func (m mysqlPersist) deleteGif(userID, documentID int64) error {
	_, err := m.db.Exec(`DELETE FROM tellomi_saved_gifs WHERE user_id=? AND document_id=?`, userID, documentID)
	return err
}

func (m mysqlPersist) findDocument(id int64) (*Item, bool) {
	var access int64
	var size int
	var mime, name string
	err := m.db.QueryRow(`SELECT access_hash, file_size, mime_type, uploaded_file_name FROM documents WHERE document_id=? LIMIT 1`, id).Scan(&access, &size, &mime, &name)
	if err != nil {
		return nil, false
	}
	if name == "" {
		name = "sticker"
	}
	item := &Item{
		DocumentID: id,
		AccessHash: access,
		MIME:       mime,
		FileName:   name,
		Size:       int32(size),
		Width:      512,
		Height:     512,
	}
	switch strings.ToLower(mime) {
	case "application/x-tgsticker":
		item.Animated = true
	case "video/webm", "video/mp4":
		item.Video = true
	}
	return item, true
}
