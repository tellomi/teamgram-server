package secretchat

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"sync"

	_ "github.com/go-sql-driver/mysql"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/msgexpiry"
	"github.com/zeromicro/go-zero/core/logx"
)

// qts updates are per user, not per auth key. A client asks for qts greater
// than the cursor it already applied. https://core.telegram.org/api/updates
//
// The queue and the chat row are the shared state. Two phone processes and a
// restart read the same rows. Typing is not queued: it is live only.

const schema = `
CREATE TABLE IF NOT EXISTS secret_chats (
  chat_id INT NOT NULL,
  access_hash BIGINT NOT NULL,
  date INT NOT NULL,
  admin_id BIGINT NOT NULL,
  participant_id BIGINT NOT NULL,
  g_a MEDIUMBLOB NOT NULL,
  g_b MEDIUMBLOB NULL,
  key_fingerprint BIGINT NOT NULL DEFAULT 0,
  phase TINYINT NOT NULL,
  history_deleted TINYINT NOT NULL DEFAULT 0,
  ttl_period INT NOT NULL DEFAULT 0,
  PRIMARY KEY (chat_id),
  KEY idx_secret_chats_admin (admin_id),
  KEY idx_secret_chats_participant (participant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS secret_qts_state (
  user_id BIGINT NOT NULL,
  qts INT NOT NULL,
  PRIMARY KEY (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS secret_qts (
  user_id BIGINT NOT NULL,
  qts INT NOT NULL,
  update_data MEDIUMTEXT NOT NULL,
  PRIMARY KEY (user_id, qts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS secret_sent (
  chat_id INT NOT NULL,
  random_id BIGINT NOT NULL,
  date INT NOT NULL,
  service TINYINT NOT NULL,
  file_data TEXT NULL,
  PRIMARY KEY (chat_id, random_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS secret_files (
  file_id BIGINT NOT NULL,
  chat_id INT NOT NULL,
  file_data TEXT NOT NULL,
  PRIMARY KEY (file_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
`

// Backend is the shared secret-chat table. Memory is the default. MySQL is
// installed with UseDSN so every process sees the same chats and qts queue.
type Backend interface {
	SaveChat(c *chat) error
	Chat(id int32) (*chat, bool)
	ChatsFor(userID int64) []*chat
	DeleteChat(id int32) error
	SaveSent(key sentKey, rec sentRec) error
	Sent(key sentKey) (sentRec, bool)
	SentFiles(chatID int32) []int64
	DeleteSent(chatID int32) error
	SaveFile(id int64, rec keptFile) error
	File(id int64) (keptFile, bool)
	Append(userID int64, upd *mtproto.Update) (int32, error)
	Since(userID int64, after, limit int32) (msgs []*mtproto.EncryptedMessage, others []*mtproto.Update, last int32, more bool)
	Current(userID int64) int32
}

type memBackend struct {
	mu    sync.Mutex
	chats map[int32]*chat
	sent  map[sentKey]sentRec
	files map[int64]keptFile
	qts   map[int64]int32
	queue map[int64][]queued
}

type queued struct {
	qts int32
	raw []byte
}

func newMem() *memBackend {
	return &memBackend{
		chats: map[int32]*chat{},
		sent:  map[sentKey]sentRec{},
		files: map[int64]keptFile{},
		qts:   map[int64]int32{},
		queue: map[int64][]queued{},
	}
}

func (m *memBackend) clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.chats = map[int32]*chat{}
	m.sent = map[sentKey]sentRec{}
	m.files = map[int64]keptFile{}
	m.qts = map[int64]int32{}
	m.queue = map[int64][]queued{}
}

func (m *memBackend) SaveChat(c *chat) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.chats[c.id] = copyChat(c)
	return nil
}

func (m *memBackend) Chat(id int32) (*chat, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.chats[id]
	if !ok {
		return nil, false
	}
	return copyChat(c), true
}

func (m *memBackend) ChatsFor(userID int64) []*chat {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*chat
	for _, c := range m.chats {
		if c.admin == userID || c.participant == userID {
			out = append(out, copyChat(c))
		}
	}
	return out
}

func (m *memBackend) DeleteChat(id int32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.chats, id)
	return nil
}

func (m *memBackend) SaveSent(key sentKey, rec sentRec) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec.file = cloneFile(rec.file)
	m.sent[key] = rec
	return nil
}

func (m *memBackend) Sent(key sentKey) (sentRec, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.sent[key]
	if !ok {
		return sentRec{}, false
	}
	rec.file = cloneFile(rec.file)
	return rec, true
}

func (m *memBackend) SentFiles(chatID int32) []int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []int64
	seen := map[int64]bool{}
	for key, rec := range m.sent {
		if key.chat != chatID || rec.file == nil || rec.file.GetId() == 0 || seen[rec.file.GetId()] {
			continue
		}
		seen[rec.file.GetId()] = true
		out = append(out, rec.file.GetId())
	}
	return out
}

func (m *memBackend) DeleteSent(chatID int32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key := range m.sent {
		if key.chat == chatID {
			delete(m.sent, key)
		}
	}
	return nil
}

func (m *memBackend) SaveFile(id int64, rec keptFile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec.file = cloneFile(rec.file)
	m.files[id] = rec
	return nil
}

func (m *memBackend) File(id int64) (keptFile, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.files[id]
	if !ok {
		return keptFile{}, false
	}
	rec.file = cloneFile(rec.file)
	return rec, true
}

func (m *memBackend) Append(userID int64, upd *mtproto.Update) (int32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.qts[userID]++
	qts := m.qts[userID]
	stampQts(upd, qts)
	raw, err := json.Marshal(upd)
	if err != nil {
		return 0, err
	}
	m.queue[userID] = append(m.queue[userID], queued{qts: qts, raw: raw})
	return qts, nil
}

func (m *memBackend) Since(userID int64, after, limit int32) ([]*mtproto.EncryptedMessage, []*mtproto.Update, int32, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return decodeSince(m.queue[userID], after, limit)
}

func (m *memBackend) Current(userID int64) int32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.qts[userID]
}

func stampQts(upd *mtproto.Update, qts int32) {
	if upd != nil && upd.GetPredicateName() == mtproto.Predicate_updateNewEncryptedMessage {
		upd.Qts = qts
	}
}

func decodeSince(rows []queued, after, limit int32) ([]*mtproto.EncryptedMessage, []*mtproto.Update, int32, bool) {
	if limit <= 0 {
		limit = 100
	}
	var msgs []*mtproto.EncryptedMessage
	var others []*mtproto.Update
	var last int32
	var n int32
	more := false
	for _, row := range rows {
		if row.qts <= after {
			continue
		}
		if n == limit {
			more = true
			break
		}
		upd := &mtproto.Update{}
		if err := json.Unmarshal(row.raw, upd); err != nil {
			continue
		}
		n++
		last = row.qts
		if upd.GetPredicateName() == mtproto.Predicate_updateNewEncryptedMessage && upd.GetMessage_ENCRYPTEDMESSAGE() != nil {
			msgs = append(msgs, upd.GetMessage_ENCRYPTEDMESSAGE())
			// The qts cursor is updates.State.qts on the difference. This
			// update also carries qts. Leaving it in other_updates makes a
			// client apply the live qts gap check (local+1 < qts) and call
			// getDifference again.
			continue
		}
		others = append(others, upd)
	}
	return msgs, others, last, more
}

func copyChat(c *chat) *chat {
	if c == nil {
		return nil
	}
	out := *c
	out.ga = clone(c.ga)
	out.gb = clone(c.gb)
	return &out
}

// sqlBackend is the multi-process table. Each phone instance and the updates
// service open their own connection to it.
type sqlBackend struct {
	db *sql.DB
}

func openSQL(dsn string) (*sqlBackend, error) {
	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(8)
	if err = conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if _, err = conn.Exec(schema); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &sqlBackend{db: conn}, nil
}

// UseDSN points every secret chat in this process at MySQL. An empty dsn
// keeps the in-process table. TEAMGRAM_MYSQL_DSN overrides dsn. A failed
// open leaves the previous backend in place.
func UseDSN(dsn string) error {
	if env := os.Getenv("TEAMGRAM_MYSQL_DSN"); env != "" {
		dsn = env
	}
	if dsn == "" {
		logx.Info("secret chat: no MySQL DSN, invites stay in this process and are lost on restart")
		return nil
	}
	be, err := openSQL(dsn)
	if err != nil {
		return err
	}
	gate.Lock()
	if prev, ok := active.(*sqlBackend); ok && prev.db != nil {
		_ = prev.db.Close()
	}
	active = be
	current = newStore(be)
	gate.Unlock()
	if err = msgexpiry.AttachSecretDB(be.db); err != nil {
		logx.Errorf("secret chat expiry: %v", err)
	}
	return nil
}

func (s *sqlBackend) SaveChat(c *chat) error {
	_, err := s.db.Exec(`
INSERT INTO secret_chats (chat_id, access_hash, date, admin_id, participant_id, g_a, g_b, key_fingerprint, phase, history_deleted, ttl_period)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  access_hash = VALUES(access_hash), date = VALUES(date), admin_id = VALUES(admin_id),
  participant_id = VALUES(participant_id), g_a = VALUES(g_a), g_b = VALUES(g_b),
  key_fingerprint = VALUES(key_fingerprint), phase = VALUES(phase), history_deleted = VALUES(history_deleted),
  ttl_period = VALUES(ttl_period)`,
		c.id, c.access, c.date, c.admin, c.participant, c.ga, c.gb, c.fingerprint, int(c.phase), boolByte(c.historyGone), c.ttl)
	return err
}

func (s *sqlBackend) Chat(id int32) (*chat, bool) {
	row := s.db.QueryRow(`SELECT chat_id, access_hash, date, admin_id, participant_id, g_a, g_b, key_fingerprint, phase, history_deleted, ttl_period FROM secret_chats WHERE chat_id = ?`, id)
	c, err := scanChat(row)
	if err != nil {
		return nil, false
	}
	return c, true
}

func (s *sqlBackend) ChatsFor(userID int64) []*chat {
	rows, err := s.db.Query(`SELECT chat_id, access_hash, date, admin_id, participant_id, g_a, g_b, key_fingerprint, phase, history_deleted, ttl_period
FROM secret_chats WHERE admin_id = ? OR participant_id = ?`, userID, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*chat
	for rows.Next() {
		c, err := scanChat(rows)
		if err != nil {
			continue
		}
		out = append(out, c)
	}
	return out
}

func (s *sqlBackend) DeleteChat(id int32) error {
	_, err := s.db.Exec(`DELETE FROM secret_chats WHERE chat_id = ?`, id)
	return err
}

func (s *sqlBackend) SaveSent(key sentKey, rec sentRec) error {
	raw, _ := json.Marshal(rec.file)
	_, err := s.db.Exec(`INSERT INTO secret_sent (chat_id, random_id, date, service, file_data) VALUES (?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE date = VALUES(date)`, key.chat, key.random, rec.date, boolByte(rec.service), string(raw))
	return err
}

func (s *sqlBackend) Sent(key sentKey) (sentRec, bool) {
	var rec sentRec
	var service int
	var raw sql.NullString
	err := s.db.QueryRow(`SELECT date, service, file_data FROM secret_sent WHERE chat_id = ? AND random_id = ?`, key.chat, key.random).Scan(&rec.date, &service, &raw)
	if err != nil {
		return sentRec{}, false
	}
	rec.service = service != 0
	if raw.Valid && raw.String != "" && raw.String != "null" {
		file := &mtproto.EncryptedFile{}
		if json.Unmarshal([]byte(raw.String), file) == nil && file.GetId() != 0 {
			rec.file = file
		}
	}
	return rec, true
}

func (s *sqlBackend) SentFiles(chatID int32) []int64 {
	rows, err := s.db.Query(`SELECT file_data FROM secret_sent WHERE chat_id = ? AND file_data IS NOT NULL`, chatID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []int64
	seen := map[int64]bool{}
	for rows.Next() {
		var raw sql.NullString
		if err := rows.Scan(&raw); err != nil || !raw.Valid {
			continue
		}
		file := &mtproto.EncryptedFile{}
		if json.Unmarshal([]byte(raw.String), file) != nil || file.GetId() == 0 || seen[file.GetId()] {
			continue
		}
		seen[file.GetId()] = true
		out = append(out, file.GetId())
	}
	return out
}

func (s *sqlBackend) DeleteSent(chatID int32) error {
	_, err := s.db.Exec(`DELETE FROM secret_sent WHERE chat_id = ?`, chatID)
	return err
}

func (s *sqlBackend) SaveFile(id int64, rec keptFile) error {
	raw, err := json.Marshal(rec.file)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO secret_files (file_id, chat_id, file_data) VALUES (?, ?, ?)
ON DUPLICATE KEY UPDATE chat_id = VALUES(chat_id), file_data = VALUES(file_data)`, id, rec.chatID, string(raw))
	return err
}

func (s *sqlBackend) File(id int64) (keptFile, bool) {
	var chatID int32
	var raw string
	if err := s.db.QueryRow(`SELECT chat_id, file_data FROM secret_files WHERE file_id = ?`, id).Scan(&chatID, &raw); err != nil {
		return keptFile{}, false
	}
	file := &mtproto.EncryptedFile{}
	if json.Unmarshal([]byte(raw), file) != nil || file.GetId() == 0 {
		return keptFile{}, false
	}
	return keptFile{file: file, chatID: chatID}, true
}

func (s *sqlBackend) Append(userID int64, upd *mtproto.Update) (int32, error) {
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`INSERT INTO secret_qts_state (user_id, qts) VALUES (?, 0) ON DUPLICATE KEY UPDATE qts = qts`, userID); err != nil {
		return 0, err
	}
	var qts int32
	if err = tx.QueryRow(`SELECT qts FROM secret_qts_state WHERE user_id = ? FOR UPDATE`, userID).Scan(&qts); err != nil {
		return 0, err
	}
	qts++
	stampQts(upd, qts)
	raw, err := json.Marshal(upd)
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(`UPDATE secret_qts_state SET qts = ? WHERE user_id = ?`, qts, userID); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(`INSERT INTO secret_qts (user_id, qts, update_data) VALUES (?, ?, ?)`, userID, qts, string(raw)); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return qts, nil
}

func (s *sqlBackend) Since(userID int64, after, limit int32) ([]*mtproto.EncryptedMessage, []*mtproto.Update, int32, bool) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT qts, update_data FROM secret_qts WHERE user_id = ? AND qts > ? ORDER BY qts LIMIT ?`, userID, after, limit+1)
	if err != nil {
		return nil, nil, 0, false
	}
	defer rows.Close()
	var queuedRows []queued
	for rows.Next() {
		var row queued
		var raw string
		if err := rows.Scan(&row.qts, &raw); err != nil {
			return nil, nil, 0, false
		}
		row.raw = []byte(raw)
		queuedRows = append(queuedRows, row)
	}
	return decodeSince(queuedRows, after, limit)
}

func (s *sqlBackend) Current(userID int64) int32 {
	var qts int32
	if err := s.db.QueryRow(`SELECT qts FROM secret_qts_state WHERE user_id = ?`, userID).Scan(&qts); err != nil {
		return 0
	}
	return qts
}

type chatScanner interface {
	Scan(dest ...any) error
}

func scanChat(row chatScanner) (*chat, error) {
	var c chat
	var phaseN int
	var history int
	var gb []byte
	if err := row.Scan(&c.id, &c.access, &c.date, &c.admin, &c.participant, &c.ga, &gb, &c.fingerprint, &phaseN, &history, &c.ttl); err != nil {
		return nil, err
	}
	c.phase = phase(phaseN)
	c.historyGone = history != 0
	c.gb = gb
	return &c, nil
}

func boolByte(v bool) int {
	if v {
		return 1
	}
	return 0
}
