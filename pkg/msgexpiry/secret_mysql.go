package msgexpiry

import (
	"database/sql"
)

const secretExpirySchema = `
CREATE TABLE IF NOT EXISTS secret_message_expiry (
  chat_id int NOT NULL,
  msg_id int NOT NULL,
  date2 int NOT NULL DEFAULT 0,
  ttl_period int NOT NULL DEFAULT 0,
  file_id bigint NOT NULL DEFAULT 0,
  swept tinyint NOT NULL DEFAULT 0,
  user_a bigint NOT NULL DEFAULT 0,
  user_b bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (chat_id, msg_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

// mysqlSecret persists KindSecret rows. It does not read messages,
// channel_messages, or backups.
type mysqlSecret struct {
	db *sql.DB
}

// AttachSecretDB creates the secret expiry table and loads it into the
// process ledger. A nil db leaves the memory ledger unchanged. Private and
// view-once rows are not stored here.
func AttachSecretDB(db *sql.DB) error {
	if db == nil {
		return nil
	}
	if _, err := db.Exec(secretExpirySchema); err != nil {
		return err
	}
	s := &mysqlSecret{db: db}
	SetSecretSink(s)
	rows, err := s.load()
	if err != nil {
		return err
	}
	led := Default()
	for _, r := range rows {
		led.Register(r)
	}
	return nil
}

func (s *mysqlSecret) Put(r Record) {
	if s == nil || s.db == nil || r.Kind != KindSecret || r.ID == 0 {
		return
	}
	a, b := int64(0), int64(0)
	if len(r.Users) > 0 {
		a = r.Users[0]
	}
	if len(r.Users) > 1 {
		b = r.Users[1]
	}
	_, _ = s.db.Exec(`
INSERT INTO secret_message_expiry (chat_id, msg_id, date2, ttl_period, file_id, swept, user_a, user_b)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  file_id = IF(file_id = 0, VALUES(file_id), file_id),
  swept = IF(swept = 1, 1, VALUES(swept))`,
		r.Scope, r.ID, r.Date, r.TTL, r.FileID, boolByte(r.Deleted), a, b)
}

func (s *mysqlSecret) MarkSwept(scope int64, id int32) {
	if s == nil || s.db == nil {
		return
	}
	_, _ = s.db.Exec(`UPDATE secret_message_expiry SET swept = 1 WHERE chat_id = ? AND msg_id = ?`, scope, id)
}

func (s *mysqlSecret) All() []Record {
	rows, err := s.load()
	if err != nil {
		return nil
	}
	return rows
}

func (s *mysqlSecret) load() ([]Record, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.Query(`SELECT chat_id, msg_id, date2, ttl_period, file_id, swept, user_a, user_b FROM secret_message_expiry`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		var r Record
		var swept int
		var a, b int64
		r.Kind = KindSecret
		if err = rows.Scan(&r.Scope, &r.ID, &r.Date, &r.TTL, &r.FileID, &swept, &a, &b); err != nil {
			return nil, err
		}
		r.Deleted = swept != 0
		if a != 0 {
			r.Users = append(r.Users, a)
		}
		if b != 0 {
			r.Users = append(r.Users, b)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func boolByte(v bool) int {
	if v {
		return 1
	}
	return 0
}
