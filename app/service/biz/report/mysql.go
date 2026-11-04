package report

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
)

const schemaReports = `
CREATE TABLE IF NOT EXISTS reports (
  id BIGINT NOT NULL AUTO_INCREMENT,
  source VARCHAR(64) NOT NULL,
  reporter_user_id BIGINT NOT NULL,
  lang_code VARCHAR(32) NOT NULL DEFAULT '',
  peer_type INT NOT NULL DEFAULT 0,
  peer_id BIGINT NOT NULL DEFAULT 0,
  subject_user_id BIGINT NOT NULL DEFAULT 0,
  target_key VARCHAR(191) NOT NULL,
  reason VARCHAR(64) NOT NULL DEFAULT '',
  message TEXT,
  object_ids VARCHAR(255) NOT NULL DEFAULT '',
  photo_id BIGINT NOT NULL DEFAULT 0,
  reaction_peer_id BIGINT NOT NULL DEFAULT 0,
  evidence_key VARCHAR(255) NOT NULL DEFAULT '',
  status VARCHAR(16) NOT NULL DEFAULT 'needs_human',
  legacy_story_id BIGINT NULL,
  dedupe_key VARCHAR(191) NULL,
  created_at INT NOT NULL,
  decided_at INT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY reports_legacy_story (legacy_story_id),
  UNIQUE KEY reports_dedupe (dedupe_key),
  KEY reports_reporter_time (reporter_user_id, created_at),
  KEY reports_target (target_key, status),
  KEY reports_status (status, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

const schemaNotices = `
CREATE TABLE IF NOT EXISTS report_notices (
  target_key VARCHAR(191) NOT NULL,
  user_id BIGINT NOT NULL,
  kind VARCHAR(32) NOT NULL,
  decision VARCHAR(16) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (target_key, user_id, kind, decision)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

const reportCols = `id, source, reporter_user_id, lang_code, peer_type, peer_id, subject_user_id, target_key, reason, IFNULL(message,''), object_ids, photo_id, reaction_peer_id, evidence_key, status, IFNULL(legacy_story_id,0), IFNULL(dedupe_key,''), created_at, decided_at`

type mysqlStore struct {
	db *sql.DB
}

func newMySQLStore(db *sql.DB) *mysqlStore {
	return &mysqlStore{db: db}
}

// Users reads discover_subject. The map key is base64url of the 16-byte sub.
// A missing table is an empty result: nobody has been recorded yet.
func (m *mysqlStore) Users(ctx context.Context, subs [][]byte) (map[string]int64, error) {
	out := map[string]int64{}
	if m == nil || m.db == nil || len(subs) == 0 {
		return out, nil
	}
	args := make([]any, len(subs))
	for i, sub := range subs {
		args[i] = sub
	}
	rows, err := m.db.QueryContext(ctx, `
SELECT sub, user_id FROM discover_subject WHERE sub IN (`+placeholders(len(subs))+`)`, args...)
	if err != nil {
		if isNoTable(err) {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var sub []byte
		var id int64
		if err = rows.Scan(&sub, &id); err != nil {
			return nil, err
		}
		if len(sub) != 16 || id <= 0 {
			continue
		}
		out[base64.RawURLEncoding.EncodeToString(sub)] = id
	}
	return out, rows.Err()
}

func ensureSchema(ctx context.Context, db *sql.DB) error {
	for _, stmt := range []string{schemaReports, schemaNotices} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	// Tables created before dedupe_key existed still need the column.
	if _, err := db.ExecContext(ctx, `ALTER TABLE reports ADD COLUMN dedupe_key VARCHAR(191) NULL`); err != nil && !isAlready(err) {
		return err
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE reports ADD UNIQUE KEY reports_dedupe (dedupe_key)`); err != nil && !isAlready(err) {
		return err
	}
	return nil
}

func (s *mysqlStore) Insert(ctx context.Context, row *Row) error {
	var legacy any
	if row.LegacyStoryID != 0 {
		legacy = row.LegacyStoryID
	}
	var dedupe any
	if row.DedupeKey != "" {
		dedupe = row.DedupeKey
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO reports (
  source, reporter_user_id, lang_code, peer_type, peer_id, subject_user_id,
  target_key, reason, message, object_ids, photo_id, reaction_peer_id,
  evidence_key, status, legacy_story_id, dedupe_key, created_at, decided_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		row.Source, row.Reporter, row.Lang, row.PeerType, row.PeerID, row.SubjectUserID,
		row.TargetKey, row.Reason, row.Message, joinIDs(row.ObjectIDs), row.PhotoID, row.ReactionPeerID,
		row.EvidenceKey, row.Status, legacy, dedupe, row.CreatedAt, row.DecidedAt)
	if isDup(err) && row.DedupeKey != "" {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	row.ID = id
	return nil
}

func (s *mysqlStore) Get(ctx context.Context, id int64) (*Row, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+reportCols+` FROM reports WHERE id=?`, id)
	got, err := scanReport(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return got, err
}

func (s *mysqlStore) ListAll(ctx context.Context) ([]*Row, error) {
	return s.query(ctx, `SELECT `+reportCols+` FROM reports ORDER BY id`)
}

func (s *mysqlStore) ListByTarget(ctx context.Context, key string) ([]*Row, error) {
	return s.query(ctx, `SELECT `+reportCols+` FROM reports WHERE target_key=? ORDER BY id`, key)
}

func (s *mysqlStore) ListPending(ctx context.Context, limit int) ([]*Row, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.query(ctx, `SELECT `+reportCols+` FROM reports WHERE status IN (?, ?) ORDER BY id LIMIT ?`, StatusPending, StatusNeedsHuman, limit)
}

func (s *mysqlStore) Recent(ctx context.Context, reporter int64, since int32) (int, int32, error) {
	var count int
	var oldest sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*), MIN(created_at) FROM reports WHERE reporter_user_id=? AND created_at>=?`, reporter, since).Scan(&count, &oldest)
	if err != nil {
		return 0, 0, err
	}
	if !oldest.Valid {
		return count, 0, nil
	}
	return count, int32(oldest.Int64), nil
}

func (s *mysqlStore) UpdateStatus(ctx context.Context, ids []int64, status string, decidedAt int32) error {
	if len(ids) == 0 {
		return nil
	}
	q := `UPDATE reports SET status=?, decided_at=? WHERE id IN (` + placeholders(len(ids)) + `)`
	args := make([]any, 0, len(ids)+2)
	args = append(args, status, decidedAt)
	for _, id := range ids {
		args = append(args, id)
	}
	_, err := s.db.ExecContext(ctx, q, args...)
	return err
}

func (s *mysqlStore) UpdateEvidence(ctx context.Context, id int64, key string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE reports SET evidence_key=? WHERE id=?`, key, id)
	return err
}

func (s *mysqlStore) FindDedupe(ctx context.Context, key string) (*Row, error) {
	if key == "" {
		return nil, nil
	}
	row := s.db.QueryRowContext(ctx, `SELECT `+reportCols+` FROM reports WHERE dedupe_key=? ORDER BY id LIMIT 1`, key)
	got, err := scanReport(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return got, err
}

func (s *mysqlStore) HasLegacy(ctx context.Context, storyID int64) (bool, error) {
	if storyID == 0 {
		return false, nil
	}
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM reports WHERE legacy_story_id=?`, storyID).Scan(&n)
	return n > 0, err
}

func (s *mysqlStore) ClaimNotice(ctx context.Context, targetKey string, userID int64, kind, decision string) (bool, error) {
	_, err := s.db.ExecContext(ctx, `INSERT INTO report_notices (target_key, user_id, kind, decision) VALUES (?,?,?,?)`, targetKey, userID, kind, decision)
	if isDup(err) {
		return false, nil
	}
	return err == nil, err
}

func (s *mysqlStore) ReleaseNotice(ctx context.Context, targetKey string, userID int64, kind, decision string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM report_notices WHERE target_key=? AND user_id=? AND kind=? AND decision=?`, targetKey, userID, kind, decision)
	return err
}

func (s *mysqlStore) query(ctx context.Context, q string, args ...any) ([]*Row, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Row
	for rows.Next() {
		got, err := scanReport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, got)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanReport(row scanner) (*Row, error) {
	var (
		got      Row
		peerType int
		objects  string
	)
	err := row.Scan(
		&got.ID, &got.Source, &got.Reporter, &got.Lang, &peerType, &got.PeerID, &got.SubjectUserID,
		&got.TargetKey, &got.Reason, &got.Message, &objects, &got.PhotoID, &got.ReactionPeerID,
		&got.EvidenceKey, &got.Status, &got.LegacyStoryID, &got.DedupeKey, &got.CreatedAt, &got.DecidedAt,
	)
	if err != nil {
		return nil, err
	}
	got.PeerType = int32(peerType)
	got.ObjectIDs = splitIDs(objects)
	return &got, nil
}

func placeholders(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ",")
}

func isDup(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

func isAlready(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && (me.Number == 1060 || me.Number == 1061)
}

func isNoTable(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1146
}

// Load reads message text and media keys for a report whose caller did not
// pass evidence. A missing table or row leaves the evidence empty.
func (s *mysqlStore) Load(ctx context.Context, in Submission) (Evidence, error) {
	var ev Evidence
	switch in.Source {
	case SourceMessages, SourceReaction, SourceChannelSpam, SourceSpam:
	default:
		return ev, nil
	}
	if in.PeerType == mtproto.PEER_CHANNEL || in.Source == SourceChannelSpam {
		for _, id := range in.ObjectIDs {
			var raw []byte
			err := s.db.QueryRowContext(ctx, `SELECT message_data FROM channel_messages WHERE channel_id=? AND message_id=? AND deleted=0`, in.PeerID, id).Scan(&raw)
			if err != nil {
				if isNoTable(err) {
					return Evidence{}, nil
				}
				continue
			}
			msg := &mtproto.Message{}
			if proto.Unmarshal(raw, msg) == nil {
				ev.Items = append(ev.Items, ContentFromMessage(int64(id), msg))
			}
		}
		return ev, nil
	}
	for _, id := range in.ObjectIDs {
		var text string
		var raw []byte
		err := s.db.QueryRowContext(ctx, `SELECT message, message_data FROM messages WHERE user_id=? AND peer_type=? AND peer_id=? AND user_message_box_id=? AND deleted=0 LIMIT 1`,
			in.Reporter, in.PeerType, in.PeerID, id).Scan(&text, &raw)
		if err != nil {
			if isNoTable(err) {
				return Evidence{}, nil
			}
			continue
		}
		ev.Items = append(ev.Items, contentFromMessageJSON(int64(id), text, raw))
	}
	return ev, nil
}

func contentFromMessageJSON(id int64, text string, raw []byte) Content {
	c := Content{Kind: "message", ID: id, Text: text}
	if len(raw) == 0 || raw[0] != '{' {
		return c
	}
	var body struct {
		Message string `json:"message"`
		Media   struct {
			Document struct {
				ID   int64  `json:"id"`
				Mime string `json:"mime_type"`
			} `json:"document"`
			PhotoFlag struct {
				ID int64 `json:"id"`
			} `json:"photo_FLAGPHOTO"`
			Photo struct {
				ID int64 `json:"id"`
			} `json:"photo"`
		} `json:"media"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return c
	}
	if c.Text == "" {
		c.Text = body.Message
	}
	if body.Media.PhotoFlag.ID != 0 {
		c.MediaKeys = append(c.MediaKeys, "photos/"+itoa(body.Media.PhotoFlag.ID))
	} else if body.Media.Photo.ID != 0 {
		c.MediaKeys = append(c.MediaKeys, "photos/"+itoa(body.Media.Photo.ID))
	}
	if body.Media.Document.ID != 0 {
		class := "documents"
		if strings.HasPrefix(strings.ToLower(body.Media.Document.Mime), "video/") {
			class = "videos"
		}
		c.MediaKeys = append(c.MediaKeys, class+"/"+itoa(body.Media.Document.ID))
	}
	return c
}

// langDB reads auths.lang_code for the user's newest session, then devices.
type langDB struct{ db *sql.DB }

func (l langDB) Lang(ctx context.Context, userID int64) string {
	if l.db == nil || userID == 0 {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var code string
	err := l.db.QueryRowContext(ctx, `
SELECT a.lang_code
FROM auth_users u
JOIN auths a ON a.auth_key_id = u.auth_key_id
WHERE u.user_id=? AND u.deleted=0 AND a.deleted=0 AND a.lang_code<>''
ORDER BY a.date_active DESC
LIMIT 1`, userID).Scan(&code)
	if err == nil && strings.TrimSpace(code) != "" {
		return strings.TrimSpace(code)
	}
	_ = l.db.QueryRowContext(ctx, `SELECT lang_code FROM devices WHERE user_id=? AND lang_code<>'' ORDER BY updated_at DESC LIMIT 1`, userID).Scan(&code)
	return strings.TrimSpace(code)
}
