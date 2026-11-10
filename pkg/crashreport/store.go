package crashreport

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	_ "github.com/go-sql-driver/mysql"
)

// ErrConflict means 777006 or @TellomiBot already belongs to a different account.
var ErrConflict = errors.New("crash report bot id or username belongs to a different account")

// Row is one stored report. StorageKey is the full object key
// (crash-reports/YYYY/MM/DD/...).
type Row struct {
	ID           int64     `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	SenderUserID int64     `json:"sender_user_id"`
	MsgID        int32     `json:"msg_id"`
	Kind         string    `json:"kind"`
	Filename     string    `json:"filename"`
	Caption      string    `json:"caption"`
	Message      string    `json:"message"`
	Device       string    `json:"device"`
	App          string    `json:"app"`
	FileSize     int64     `json:"file_size"`
	ContentType  string    `json:"content_type"`
	StorageClass string    `json:"storage_class"`
	StorageKey   string    `json:"storage_key"`
	DownloadURL  string    `json:"download_url,omitempty"`
}

// Query filters a list. Zero times and a zero sender mean no bound.
type Query struct {
	From   time.Time
	To     time.Time
	Sender int64
	Device string
	Limit  int
}

type docSource struct {
	Class string
	Path  string
	Size  int64
	Mime  string
	Name  string
}

// OpenMySQL opens the teamgram database. An empty DSN returns nil, nil.
func OpenMySQL(dsn string) (*sql.DB, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, nil
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetConnMaxLifetime(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Ensure creates @TellomiBot and the crash_reports table when they are absent.
// It does not delete or rewrite any other user. A second call changes nothing.
func Ensure(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("mysql is not configured")
	}
	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		return err
	}
	var taken string
	err := db.QueryRowContext(ctx, `
SELECT username FROM username
WHERE username = ? AND NOT (peer_type = 2 AND peer_id = ?)
LIMIT 1`, DefaultUsername, AccountID).Scan(&taken)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if taken != "" {
		return ErrConflict
	}
	var nameOwner int64
	err = db.QueryRowContext(ctx, `
SELECT id FROM users WHERE username = ? AND id <> ? LIMIT 1`, DefaultUsername, AccountID).Scan(&nameOwner)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if nameOwner != 0 {
		return ErrConflict
	}
	var id int64
	var userType, isBot int
	var username string
	err = db.QueryRowContext(ctx, `
SELECT id, user_type, is_bot, username FROM users WHERE id = ?`, AccountID).Scan(&id, &userType, &isBot, &username)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && !ourBot(userType, isBot, username) {
		return ErrConflict
	}
	var other int64
	err = db.QueryRowContext(ctx, `
SELECT bot_id FROM bots WHERE token = ? AND bot_id <> ? LIMIT 1`, placeholderToken, AccountID).Scan(&other)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if other != 0 {
		return ErrConflict
	}
	if _, err = db.ExecContext(ctx, `
INSERT IGNORE INTO users (id, user_type, access_hash, secret_key_id, first_name, last_name, username, phone, country_code, verified, support, about, is_bot)
VALUES (?, 3, 8100000000000777006, 0, ?, '', ?, NULL, '', 1, 0, ?, 1)`,
		AccountID, DisplayName, DefaultUsername, "把崩溃日志发到这里。"); err != nil {
		return err
	}
	if _, err = db.ExecContext(ctx, `
UPDATE users
SET username = ?, first_name = ?
WHERE id = ? AND user_type = 3 AND is_bot = 1 AND username = ''`,
		DefaultUsername, DisplayName, AccountID); err != nil {
		return err
	}
	if _, err = db.ExecContext(ctx, `
INSERT IGNORE INTO bots (bot_id, bot_type, creator_user_id, token, description, bot_chat_history, bot_nochats, bot_info_version)
VALUES (?, 0, 0, ?, 'Tellomi crash reports', 0, 1, 1)`, AccountID, placeholderToken); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
INSERT IGNORE INTO username (username, peer_type, peer_id, editable, active, order2, deleted)
VALUES (?, 2, ?, 1, 1, 2, 0)`, DefaultUsername, AccountID)
	return err
}

func ourBot(userType, isBot int, username string) bool {
	if userType != 3 || isBot != 1 {
		return false
	}
	return username == "" || strings.EqualFold(username, DefaultUsername)
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS crash_reports (
  id BIGINT NOT NULL AUTO_INCREMENT,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  sender_user_id BIGINT NOT NULL,
  msg_id INT NOT NULL,
  kind VARCHAR(16) NOT NULL DEFAULT 'text',
  filename VARCHAR(255) NOT NULL DEFAULT '',
  caption VARCHAR(4096) NOT NULL DEFAULT '',
  message_text TEXT,
  device_info VARCHAR(512) NOT NULL DEFAULT '',
  app_info VARCHAR(512) NOT NULL DEFAULT '',
  file_size BIGINT NOT NULL DEFAULT 0,
  content_type VARCHAR(128) NOT NULL DEFAULT '',
  storage_class VARCHAR(64) NOT NULL DEFAULT 'crash-reports',
  storage_key VARCHAR(512) NOT NULL DEFAULT '',
  PRIMARY KEY (id),
  UNIQUE KEY uniq_crash_msg (sender_user_id, msg_id),
  KEY idx_crash_created (created_at),
  KEY idx_crash_sender (sender_user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

func (s *Service) save(ctx context.Context, row Row) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("crash report store is not configured")
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO crash_reports
  (sender_user_id, msg_id, kind, filename, caption, message_text, device_info, app_info, file_size, content_type, storage_class, storage_key)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  filename = VALUES(filename),
  caption = VALUES(caption),
  message_text = VALUES(message_text),
  device_info = VALUES(device_info),
  app_info = VALUES(app_info),
  file_size = VALUES(file_size),
  content_type = VALUES(content_type),
  storage_class = VALUES(storage_class),
  storage_key = VALUES(storage_key)`,
		row.SenderUserID, row.MsgID, row.Kind, clip(row.Filename, 255), clip(row.Caption, 4096), row.Message,
		clip(row.Device, 512), clip(row.App, 512), row.FileSize, clip(row.ContentType, 128), row.StorageClass, clip(row.StorageKey, 512))
	return err
}

func (s *Service) document(ctx context.Context, id int64) (docSource, error) {
	var src docSource
	if id == 0 || s == nil || s.db == nil {
		return src, fmt.Errorf("document %d is not stored", id)
	}
	err := s.db.QueryRowContext(ctx, `
SELECT b.storage_class, b.file_path, b.file_size
FROM media_blob_refs r
JOIN media_blobs b ON b.id = r.blob_id
WHERE r.ref_kind = 'document' AND r.ref_id = ? AND b.file_path <> ''
LIMIT 1`, id).Scan(&src.Class, &src.Path, &src.Size)
	if err != nil && !errors.Is(err, sql.ErrNoRows) && !missingTable(err) {
		return src, err
	}
	var path, mime, name string
	var size int64
	err = s.db.QueryRowContext(ctx, `
SELECT file_path, file_size, mime_type, uploaded_file_name
FROM documents WHERE document_id = ?`, id).Scan(&path, &size, &mime, &name)
	if errors.Is(err, sql.ErrNoRows) {
		if src.Path == "" {
			return src, fmt.Errorf("document %d is not stored", id)
		}
		return src, nil
	}
	if err != nil {
		if src.Path != "" && missingTable(err) {
			return src, nil
		}
		return src, err
	}
	src.Mime = mime
	src.Name = name
	if src.Path == "" {
		src.Class = "documents"
		src.Path = path
		src.Size = size
	} else if src.Size == 0 {
		src.Size = size
	}
	return src, nil
}

func (s *Service) list(ctx context.Context, q Query) ([]Row, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("crash report store is not configured")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	device := strings.TrimSpace(q.Device)
	like := ""
	if device != "" {
		like = "%" + escapeLike(clip(device, 128)) + "%"
	}
	var fromUnix, toUnix int64
	if !q.From.IsZero() {
		fromUnix = q.From.UTC().Unix()
	}
	if !q.To.IsZero() {
		toUnix = q.To.UTC().Unix()
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, created_at, sender_user_id, msg_id, kind, filename, caption, IFNULL(message_text,''), device_info, app_info, file_size, content_type, storage_class, storage_key
FROM crash_reports
WHERE (? = 0 OR created_at >= FROM_UNIXTIME(?))
  AND (? = 0 OR created_at <= FROM_UNIXTIME(?))
  AND (? = 0 OR sender_user_id = ?)
  AND (? = '' OR device_info LIKE ? OR app_info LIKE ? OR filename LIKE ? OR caption LIKE ? OR message_text LIKE ?)
ORDER BY created_at DESC, id DESC
LIMIT ?`,
		fromUnix, fromUnix,
		toUnix, toUnix,
		q.Sender, q.Sender,
		like, like, like, like, like, like,
		limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var row Row
		if err = rows.Scan(&row.ID, &row.CreatedAt, &row.SenderUserID, &row.MsgID, &row.Kind, &row.Filename, &row.Caption, &row.Message, &row.Device, &row.App, &row.FileSize, &row.ContentType, &row.StorageClass, &row.StorageKey); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	i := n
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	return s[:i]
}

func missingTable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "1146") || strings.Contains(msg, "doesn't exist") || strings.Contains(msg, "does not exist")
}
