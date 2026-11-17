package book

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/pkg/mysqlpool"
	"github.com/teamgram/teamgram-server/pkg/mysqltls"
)

const collectionSchema = `
CREATE TABLE IF NOT EXISTS tellomi_star_gift_collections (
  peer_key VARCHAR(80) NOT NULL,
  collection_id INT NOT NULL,
  title VARCHAR(256) NOT NULL,
  sort_pos INT NOT NULL,
  PRIMARY KEY (peer_key, collection_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS tellomi_star_gift_collection_seq (
  peer_key VARCHAR(80) NOT NULL,
  next_id INT NOT NULL,
  PRIMARY KEY (peer_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
`

type mysqlCollections struct {
	db *sql.DB
}

func openCollectionMySQL(dsn string) (*mysqlCollections, error) {
	db, err := mysqltls.Open(dsn)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = mysqlpool.Discard(db)
		return nil, err
	}
	for _, stmt := range strings.Split(collectionSchema, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err = db.ExecContext(ctx, stmt); err != nil {
			_ = mysqlpool.Discard(db)
			return nil, err
		}
	}
	return &mysqlCollections{db: db}, nil
}

func (m *mysqlCollections) Load(ctx context.Context) (map[string][]giftCollection, map[string]int32, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT peer_key, collection_id, title FROM tellomi_star_gift_collections ORDER BY peer_key, sort_pos, collection_id`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out := map[string][]giftCollection{}
	for rows.Next() {
		var key, title string
		var id int32
		if err = rows.Scan(&key, &id, &title); err != nil {
			return nil, nil, err
		}
		out[key] = append(out[key], giftCollection{id: id, title: title})
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}
	nextRows, err := m.db.QueryContext(ctx, `SELECT peer_key, next_id FROM tellomi_star_gift_collection_seq`)
	if err != nil {
		return nil, nil, err
	}
	defer nextRows.Close()
	next := map[string]int32{}
	for nextRows.Next() {
		var key string
		var id int32
		if err = nextRows.Scan(&key, &id); err != nil {
			return nil, nil, err
		}
		next[key] = id
	}
	if err = nextRows.Err(); err != nil {
		return nil, nil, err
	}
	for key, list := range out {
		var maxID int32
		for _, row := range list {
			if row.id > maxID {
				maxID = row.id
			}
		}
		if next[key] < maxID {
			next[key] = maxID
		}
	}
	return out, next, nil
}

func (m *mysqlCollections) Save(ctx context.Context, key string, rows []giftCollection, next int32) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `DELETE FROM tellomi_star_gift_collections WHERE peer_key=?`, key); err != nil {
		return err
	}
	for i, row := range rows {
		if _, err = tx.ExecContext(ctx, `INSERT INTO tellomi_star_gift_collections (peer_key, collection_id, title, sort_pos) VALUES (?,?,?,?)`, key, row.id, row.title, i); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO tellomi_star_gift_collection_seq (peer_key, next_id) VALUES (?,?)
ON DUPLICATE KEY UPDATE next_id=VALUES(next_id)`, key, next); err != nil {
		return err
	}
	return tx.Commit()
}
