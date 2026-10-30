package reaction

import (
	"context"
	"database/sql"
	"strings"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS message_reactions (
  id BIGINT NOT NULL AUTO_INCREMENT,
  dialog_id1 BIGINT NOT NULL,
  dialog_id2 BIGINT NOT NULL,
  dialog_message_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  emoticon VARCHAR(32) NOT NULL,
  big TINYINT NOT NULL DEFAULT 0,
  date2 INT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uniq_message_reaction (dialog_id1, dialog_id2, dialog_message_id, user_id, emoticon),
  KEY idx_message_reaction_msg (dialog_id1, dialog_id2, dialog_message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS message_reaction_boxes (
  user_id BIGINT NOT NULL,
  peer_type INT NOT NULL,
  peer_id BIGINT NOT NULL,
  msg_id INT NOT NULL,
  dialog_id1 BIGINT NOT NULL,
  dialog_id2 BIGINT NOT NULL,
  dialog_message_id BIGINT NOT NULL,
  PRIMARY KEY (user_id, peer_type, peer_id, msg_id),
  KEY idx_message_reaction_box_dialog (dialog_id1, dialog_id2, dialog_message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
`

// EnsureSchema creates message_reactions and the per-user box index.
func EnsureSchema(ctx context.Context, db *sql.DB) error {
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

type mysqlStore struct {
	db *sql.DB
}

// NewMySQL stores reactions in message_reactions. db must already be open.
func NewMySQL(db *sql.DB) Store {
	return &mysqlStore{db: db}
}

func (s *mysqlStore) Replace(ctx context.Context, key Key, actor int64, emoticons []string, big bool, boxes []Box) ([]Row, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM message_reactions WHERE dialog_id1=? AND dialog_id2=? AND dialog_message_id=? AND user_id=?`,
		key.DialogID1, key.DialogID2, key.DialogMessageID, actor); err != nil {
		return nil, err
	}
	now := int32(timeNow())
	seen := map[string]bool{}
	for _, emo := range emoticons {
		if emo == "" || seen[emo] {
			continue
		}
		seen[emo] = true
		if _, err = tx.ExecContext(ctx, `INSERT INTO message_reactions (dialog_id1, dialog_id2, dialog_message_id, user_id, emoticon, big, date2) VALUES (?,?,?,?,?,?,?)`,
			key.DialogID1, key.DialogID2, key.DialogMessageID, actor, emo, boolInt(big), now); err != nil {
			return nil, err
		}
	}
	for _, b := range boxes {
		if b.UserID == 0 || b.MsgID == 0 {
			continue
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO message_reaction_boxes (user_id, peer_type, peer_id, msg_id, dialog_id1, dialog_id2, dialog_message_id)
VALUES (?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE dialog_id1=VALUES(dialog_id1), dialog_id2=VALUES(dialog_id2), dialog_message_id=VALUES(dialog_message_id)`,
			b.UserID, b.PeerType, b.PeerID, b.MsgID, key.DialogID1, key.DialogID2, key.DialogMessageID); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return s.List(ctx, key)
}

func (s *mysqlStore) List(ctx context.Context, key Key) ([]Row, error) {
	grouped, err := s.ListMany(ctx, []Key{key})
	if err != nil {
		return nil, err
	}
	return grouped[key], nil
}

func (s *mysqlStore) ListMany(ctx context.Context, keys []Key) (map[Key][]Row, error) {
	out := map[Key][]Row{}
	if len(keys) == 0 {
		return out, nil
	}
	var ph []string
	var args []any
	seen := map[Key]bool{}
	for _, k := range keys {
		if seen[k] || k.DialogMessageID == 0 {
			continue
		}
		seen[k] = true
		ph = append(ph, "(?,?,?)")
		args = append(args, k.DialogID1, k.DialogID2, k.DialogMessageID)
	}
	if len(ph) == 0 {
		return out, nil
	}
	q := `SELECT dialog_id1, dialog_id2, dialog_message_id, user_id, emoticon, big, date2 FROM message_reactions WHERE (dialog_id1, dialog_id2, dialog_message_id) IN (` + strings.Join(ph, ",") + `) ORDER BY date2, id`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k Key
		var r Row
		var big int
		if err = rows.Scan(&k.DialogID1, &k.DialogID2, &k.DialogMessageID, &r.UserID, &r.Emoticon, &big, &r.Date); err != nil {
			return nil, err
		}
		r.Big = big != 0
		out[k] = append(out[k], r)
	}
	return out, rows.Err()
}

func (s *mysqlStore) KeyForBox(ctx context.Context, userID int64, peerType int32, peerID int64, msgID int32) (Key, bool, error) {
	var k Key
	err := s.db.QueryRowContext(ctx, `SELECT dialog_id1, dialog_id2, dialog_message_id FROM message_reaction_boxes WHERE user_id=? AND peer_type=? AND peer_id=? AND msg_id=?`,
		userID, peerType, peerID, msgID).Scan(&k.DialogID1, &k.DialogID2, &k.DialogMessageID)
	if err == sql.ErrNoRows {
		return Key{}, false, nil
	}
	if err != nil {
		return Key{}, false, err
	}
	return k, true, nil
}

func (s *mysqlStore) SavePts(ctx context.Context, userID int64, pts, ptsCount, updateType int32, updateJSON string, date int64) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO user_pts_updates (user_id, pts, pts_count, update_type, update_data, date2) VALUES (?,?,?,?,?,?)`,
		userID, pts, ptsCount, updateType, updateJSON, date)
	return err
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
