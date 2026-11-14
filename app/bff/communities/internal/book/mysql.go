package book

import (
	"database/sql"
	"errors"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

const communitySchema = `
CREATE TABLE IF NOT EXISTS communities (
  id bigint NOT NULL,
  access_hash bigint NOT NULL,
  owner_user_id bigint NOT NULL,
  title varchar(128) NOT NULL,
  about varchar(255) NOT NULL DEFAULT '',
  collapsed tinyint NOT NULL DEFAULT 0,
  created_at int NOT NULL,
  PRIMARY KEY (id),
  KEY communities_owner (owner_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS community_links (
  community_id bigint NOT NULL,
  peer_type int NOT NULL,
  peer_id bigint NOT NULL,
  visible tinyint NOT NULL DEFAULT 0,
  pending tinyint NOT NULL DEFAULT 0,
  requested_by bigint NOT NULL,
  created_at int NOT NULL,
  PRIMARY KEY (community_id, peer_type, peer_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS community_members (
  community_id bigint NOT NULL,
  user_id bigint NOT NULL,
  banned tinyint NOT NULL DEFAULT 0,
  PRIMARY KEY (community_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
`

// OpenMySQL loads communities from MySQL and writes later changes there.
// An empty DSN leaves the book in memory. TEAMGRAM_MYSQL_DSN overrides dsn.
func (b *Book) OpenMySQL(dsn string) error {
	if b == nil {
		return errors.New("nil community book")
	}
	if env := strings.TrimSpace(os.Getenv("TEAMGRAM_MYSQL_DSN")); env != "" {
		dsn = env
	}
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	if err = db.Ping(); err != nil {
		_ = db.Close()
		return err
	}
	if err = ensureCommunitySchema(db); err != nil {
		_ = db.Close()
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err = b.load(db); err != nil {
		_ = db.Close()
		return err
	}
	b.db = db
	return nil
}

func ensureCommunitySchema(db *sql.DB) error {
	for _, stmt := range strings.Split(communitySchema, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (b *Book) save(c *community) error {
	if b == nil || b.db == nil || c == nil {
		return nil
	}
	tx, err := b.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	collapsed := 0
	if c.collapsed {
		collapsed = 1
	}
	if _, err = tx.Exec(`INSERT INTO communities (id, access_hash, owner_user_id, title, about, collapsed, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE access_hash=VALUES(access_hash), title=VALUES(title), about=VALUES(about), collapsed=VALUES(collapsed)`,
		c.id, c.hash, c.owner, c.title, c.about, collapsed, c.date); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM community_links WHERE community_id=?`, c.id); err != nil {
		return err
	}
	for _, link := range c.links {
		vis, pend := 0, 0
		if link.visible {
			vis = 1
		}
		if link.pending {
			pend = 1
		}
		if _, err = tx.Exec(`INSERT INTO community_links (community_id, peer_type, peer_id, visible, pending, requested_by, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			c.id, link.peerType, link.peerID, vis, pend, link.requestedBy, link.date); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`DELETE FROM community_members WHERE community_id=?`, c.id); err != nil {
		return err
	}
	seen := map[int64]bool{}
	for id := range c.members {
		seen[id] = false
	}
	for id := range c.banned {
		seen[id] = true
	}
	for id, banned := range seen {
		flag := 0
		if banned {
			flag = 1
		}
		if _, err = tx.Exec(`INSERT INTO community_members (community_id, user_id, banned) VALUES (?, ?, ?)`, c.id, id, flag); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (b *Book) load(db *sql.DB) error {
	rows, err := db.Query(`SELECT id, access_hash, owner_user_id, title, about, collapsed, created_at FROM communities`)
	if err != nil {
		return err
	}
	defer rows.Close()
	byID := map[int64]*community{}
	next := idBase
	for rows.Next() {
		var c community
		var collapsed int
		if err = rows.Scan(&c.id, &c.hash, &c.owner, &c.title, &c.about, &collapsed, &c.date); err != nil {
			return err
		}
		c.collapsed = collapsed != 0
		c.members = map[int64]bool{}
		c.banned = map[int64]bool{}
		stored := c
		byID[stored.id] = &stored
		if stored.id >= next {
			next = stored.id + 1
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	links, err := db.Query(`SELECT community_id, peer_type, peer_id, visible, pending, requested_by, created_at FROM community_links`)
	if err != nil {
		return err
	}
	defer links.Close()
	for links.Next() {
		var id int64
		var link peerLink
		var vis, pend int
		if err = links.Scan(&id, &link.peerType, &link.peerID, &vis, &pend, &link.requestedBy, &link.date); err != nil {
			return err
		}
		link.visible = vis != 0
		link.pending = pend != 0
		if c := byID[id]; c != nil {
			cp := link
			c.links = append(c.links, &cp)
		}
	}
	if err = links.Err(); err != nil {
		return err
	}
	members, err := db.Query(`SELECT community_id, user_id, banned FROM community_members`)
	if err != nil {
		return err
	}
	defer members.Close()
	for members.Next() {
		var id, user int64
		var banned int
		if err = members.Scan(&id, &user, &banned); err != nil {
			return err
		}
		c := byID[id]
		if c == nil {
			continue
		}
		if banned != 0 {
			c.banned[user] = true
			continue
		}
		c.members[user] = true
	}
	if err = members.Err(); err != nil {
		return err
	}
	b.byID = byID
	b.next = next
	return nil
}
