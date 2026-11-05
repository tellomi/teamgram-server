package wipe

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/mediablob"
	"google.golang.org/protobuf/proto"
)

// The JSON paths match the functional indexes used by media retention.
const (
	exprDocumentID = "JSON_VALUE(message_data, '$.media.document.id' RETURNING SIGNED NULL ON EMPTY NULL ON ERROR)"
	exprPhotoID    = "JSON_VALUE(message_data, '$.media.photo_FLAGPHOTO.id' RETURNING SIGNED NULL ON EMPTY NULL ON ERROR)"
)

const gcSchema = `
CREATE TABLE IF NOT EXISTS account_media_gc (
  id BIGINT NOT NULL AUTO_INCREMENT,
  user_id BIGINT NOT NULL,
  kind VARCHAR(16) NOT NULL,
  media_id BIGINT NOT NULL,
  class VARCHAR(32) NOT NULL,
  object_path VARCHAR(255) NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  trashed_at BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY account_media_gc_obj (user_id, class, object_path),
  KEY account_media_gc_status (status, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
`

// SQL is the teamgram schema. Shards is the messages table count; 1 means
// the unsharded messages table.
type SQL struct {
	DB     *sql.DB
	Shards int

	channelPhotos map[int64]struct{}
	channelDocs   map[int64]struct{}
	channelErr    bool
	channelReady  bool
}

// OpenSQL opens MySQL and creates the garbage-collection queue if needed.
func OpenSQL(dsn string, shards int) (*SQL, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err = db.ExecContext(ctx, gcSchema); err != nil {
		_ = db.Close()
		return nil, err
	}
	if shards <= 1 {
		shards = 1
	}
	return &SQL{DB: db, Shards: shards}, nil
}

func (s *SQL) table(userID int64) string {
	if s.Shards <= 1 {
		return "messages"
	}
	n := userID % int64(s.Shards)
	if n < 0 {
		n = -n
	}
	return fmt.Sprintf("messages_%d", n)
}

func (s *SQL) allTables() []string {
	if s.Shards <= 1 {
		return []string{"messages"}
	}
	out := make([]string, s.Shards)
	for i := 0; i < s.Shards; i++ {
		out[i] = fmt.Sprintf("messages_%d", i)
	}
	return out
}

func (s *SQL) PrivatePeers(ctx context.Context, userID int64) ([]Peer, error) {
	seen := map[Peer]struct{}{}
	var out []Peer
	add := func(t int32, id int64) {
		if t != mtproto.PEER_SELF && t != mtproto.PEER_USER {
			return
		}
		p := Peer{Type: t, ID: id}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT peer_type, peer_id FROM dialogs
WHERE user_id=? AND deleted=0 AND peer_type IN (1,2)`, userID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t int32
		var id int64
		if err = rows.Scan(&t, &id); err != nil {
			rows.Close()
			return nil, err
		}
		add(t, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	q := `SELECT peer_type, peer_id FROM ` + s.table(userID) + `
WHERE user_id=? AND deleted=0 AND peer_type IN (1,2)
GROUP BY peer_type, peer_id`
	rows, err = s.DB.QueryContext(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t int32
		var id int64
		if err = rows.Scan(&t, &id); err != nil {
			return nil, err
		}
		add(t, id)
	}
	return out, rows.Err()
}

func (s *SQL) Boxes(ctx context.Context, userID int64, peer Peer, after int32, limit int) ([]Box, error) {
	if limit <= 0 {
		limit = DefaultBatch
	}
	q := `SELECT user_message_box_id,
COALESCE(` + exprDocumentID + `, 0),
COALESCE(` + exprPhotoID + `, 0)
FROM ` + s.table(userID) + `
WHERE user_id=? AND peer_type=? AND peer_id=? AND deleted=0 AND user_message_box_id>?
ORDER BY user_message_box_id LIMIT ?`
	rows, err := s.DB.QueryContext(ctx, q, userID, peer.Type, peer.ID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Box
	for rows.Next() {
		var b Box
		if err = rows.Scan(&b.ID, &b.DocumentID, &b.PhotoID); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *SQL) CommitDelete(ctx context.Context, userID int64, ids []int32, pts, ptsCount, updateType int32, updateJSON []byte) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	args = append(args, userID)
	for _, id := range ids {
		args = append(args, id)
	}
	q := `UPDATE ` + s.table(userID) + ` SET deleted=1 WHERE user_id=? AND user_message_box_id IN (` + ph + `) AND deleted=0`
	if _, err = tx.ExecContext(ctx, q, args...); err != nil {
		return err
	}
	if pts > 0 && len(updateJSON) > 0 {
		if _, err = tx.ExecContext(ctx, `
INSERT INTO user_pts_updates (user_id, pts, pts_count, update_type, update_data, date2)
VALUES (?,?,?,?,?,?)`, userID, pts, ptsCount, updateType, string(updateJSON), time.Now().Unix()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQL) DeleteDialog(ctx context.Context, userID int64, peer Peer) error {
	_, err := s.DB.ExecContext(ctx, `
UPDATE dialogs SET deleted=1 WHERE user_id=? AND peer_type=? AND peer_id=? AND deleted=0`,
		userID, peer.Type, peer.ID)
	return err
}

func (s *SQL) AllocPts(ctx context.Context, userID int64, n int32) (int32, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var cur sql.NullInt64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(pts),0) FROM user_pts_updates WHERE user_id=? FOR UPDATE`, userID).Scan(&cur); err != nil {
		return 0, err
	}
	end := int32(cur.Int64) + n
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return end, nil
}

func (s *SQL) BasicGroups(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT chat_id FROM chat_participants WHERE user_id=? AND state=0`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *SQL) LeaveBasic(ctx context.Context, userID, chatID, now int64) (LeftNotice, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return LeftNotice{}, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `
SELECT user_id, participant_type, invited_at, state FROM chat_participants WHERE chat_id=?`, chatID)
	if err != nil {
		return LeftNotice{}, err
	}
	var members []Member
	in := false
	for rows.Next() {
		var m Member
		if err = rows.Scan(&m.UserID, &m.Role, &m.Date, &m.State); err != nil {
			rows.Close()
			return LeftNotice{}, err
		}
		members = append(members, m)
		if m.UserID == userID && m.State == mtproto.ChatMemberStateNormal {
			in = true
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return LeftNotice{}, err
	}
	rows.Close()
	if !in {
		return LeftNotice{}, tx.Commit()
	}
	var creator int64
	if err = tx.QueryRowContext(ctx, `SELECT creator_user_id FROM chats WHERE id=?`, chatID).Scan(&creator); err != nil {
		return LeftNotice{}, err
	}
	notice := LeftNotice{ChatID: chatID}
	isCreator := creator == userID
	for _, m := range members {
		if m.UserID == userID && m.Role == mtproto.ChatMemberCreator {
			isCreator = true
		}
	}
	if isCreator {
		to, ok := PickSuccessor(members, userID)
		if !ok {
			if _, err = tx.ExecContext(ctx, `
UPDATE chat_participants SET kicked_at=0, left_at=?, state=1 WHERE chat_id=? AND user_id=? AND state=0`, now, chatID, userID); err != nil {
				return LeftNotice{}, err
			}
			if _, err = tx.ExecContext(ctx, `
UPDATE chats SET deactivated=1, participant_count=GREATEST(participant_count-1,0), version=version+1, date=? WHERE id=?`, now, chatID); err != nil {
				return LeftNotice{}, err
			}
			if err = tx.QueryRowContext(ctx, `SELECT version FROM chats WHERE id=?`, chatID).Scan(&notice.Version); err != nil {
				return LeftNotice{}, err
			}
			notice.Deactivated = true
			return notice, tx.Commit()
		}
		if _, err = tx.ExecContext(ctx, `UPDATE chats SET creator_user_id=?, version=version+1, date=? WHERE id=?`, to, now, chatID); err != nil {
			return LeftNotice{}, err
		}
		if _, err = tx.ExecContext(ctx, `
UPDATE chat_participants SET participant_type=? WHERE chat_id=? AND user_id=? AND state=0`, mtproto.ChatMemberAdmin, chatID, userID); err != nil {
			return LeftNotice{}, err
		}
		if _, err = tx.ExecContext(ctx, `
UPDATE chat_participants SET participant_type=? WHERE chat_id=? AND user_id=? AND state=0`, mtproto.ChatMemberCreator, chatID, to); err != nil {
			return LeftNotice{}, err
		}
		notice.Successor = to
	}
	res, err := tx.ExecContext(ctx, `
UPDATE chat_participants SET kicked_at=0, left_at=?, state=1 WHERE chat_id=? AND user_id=? AND state=0`, now, chatID, userID)
	if err != nil {
		return LeftNotice{}, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		if _, err = tx.ExecContext(ctx, `
UPDATE chats SET participant_count=GREATEST(participant_count-1,0), version=version+1, date=? WHERE id=?`, now, chatID); err != nil {
			return LeftNotice{}, err
		}
	}
	if err = tx.QueryRowContext(ctx, `SELECT version FROM chats WHERE id=?`, chatID).Scan(&notice.Version); err != nil {
		return LeftNotice{}, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT user_id FROM chat_participants WHERE chat_id=? AND state=0`, chatID)
	if err != nil {
		return LeftNotice{}, err
	}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return LeftNotice{}, err
		}
		notice.Members = append(notice.Members, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return LeftNotice{}, err
	}
	rows.Close()
	return notice, tx.Commit()
}

func (s *SQL) LeaveChannels(ctx context.Context, userID, now int64) ([]LeftNotice, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT p.channel_id, p.role FROM channel_participants p
JOIN channels c ON c.id=p.channel_id
WHERE p.user_id=? AND p.state=0 AND c.deleted=0`, userID)
	if err != nil {
		if missing(err) {
			return nil, nil
		}
		return nil, err
	}
	type row struct {
		id   int64
		role int32
	}
	var list []row
	for rows.Next() {
		var r row
		if err = rows.Scan(&r.id, &r.role); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, r)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	var out []LeftNotice
	for _, r := range list {
		notice, err := s.leaveChannel(ctx, userID, r.id, r.role, now)
		if err != nil {
			return out, err
		}
		if notice.ChatID != 0 {
			out = append(out, notice)
		}
	}
	return out, nil
}

func (s *SQL) leaveChannel(ctx context.Context, userID, channelID int64, role int32, now int64) (LeftNotice, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return LeftNotice{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var state int32
	err = tx.QueryRowContext(ctx, `SELECT state FROM channel_participants WHERE channel_id=? AND user_id=?`, channelID, userID).Scan(&state)
	if err == sql.ErrNoRows || state != 0 {
		return LeftNotice{}, tx.Commit()
	}
	if err != nil {
		return LeftNotice{}, err
	}
	notice := LeftNotice{ChatID: channelID, Channel: true}
	if role == 1 {
		prows, err := tx.QueryContext(ctx, `
SELECT user_id, role, date2, state FROM channel_participants WHERE channel_id=?`, channelID)
		if err != nil {
			return LeftNotice{}, err
		}
		var members []Member
		for prows.Next() {
			var m Member
			if err = prows.Scan(&m.UserID, &m.Role, &m.Date, &m.State); err != nil {
				prows.Close()
				return LeftNotice{}, err
			}
			members = append(members, m)
		}
		if err = prows.Err(); err != nil {
			prows.Close()
			return LeftNotice{}, err
		}
		prows.Close()
		to, ok := PickSuccessor(members, userID)
		if !ok {
			if _, err = tx.ExecContext(ctx, `UPDATE channel_participants SET state=1 WHERE channel_id=? AND user_id=? AND state=0`, channelID, userID); err != nil {
				return LeftNotice{}, err
			}
			var n int32
			if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM channel_participants WHERE channel_id=? AND state=0`, channelID).Scan(&n); err != nil {
				return LeftNotice{}, err
			}
			if _, err = tx.ExecContext(ctx, `UPDATE channels SET deleted=1, participants_count=?, version=version+1 WHERE id=?`, n, channelID); err != nil {
				return LeftNotice{}, err
			}
			notice.Deactivated = true
			return notice, tx.Commit()
		}
		if _, err = tx.ExecContext(ctx, `UPDATE channel_participants SET role=2 WHERE channel_id=? AND user_id=? AND state=0`, channelID, userID); err != nil {
			return LeftNotice{}, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE channel_participants SET role=1 WHERE channel_id=? AND user_id=? AND state=0`, channelID, to); err != nil {
			return LeftNotice{}, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE channels SET creator_user_id=?, version=version+1 WHERE id=?`, to, channelID); err != nil {
			return LeftNotice{}, err
		}
		notice.Successor = to
	}
	res, err := tx.ExecContext(ctx, `UPDATE channel_participants SET state=1 WHERE channel_id=? AND user_id=? AND state=0`, channelID, userID)
	if err != nil {
		return LeftNotice{}, err
	}
	changed, _ := res.RowsAffected()
	var n int32
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM channel_participants WHERE channel_id=? AND state=0`, channelID).Scan(&n); err != nil {
		return LeftNotice{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE channels SET participants_count=?, version=version+1 WHERE id=?`, n, channelID); err != nil {
		return LeftNotice{}, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT version FROM channels WHERE id=?`, channelID).Scan(&notice.Version); err != nil {
		return LeftNotice{}, err
	}
	if changed == 0 && notice.Successor == 0 {
		return LeftNotice{}, tx.Commit()
	}
	mrows, err := tx.QueryContext(ctx, `SELECT user_id FROM channel_participants WHERE channel_id=? AND state=0`, channelID)
	if err != nil {
		return LeftNotice{}, err
	}
	for mrows.Next() {
		var id int64
		if err = mrows.Scan(&id); err != nil {
			mrows.Close()
			return LeftNotice{}, err
		}
		notice.Members = append(notice.Members, id)
	}
	if err = mrows.Err(); err != nil {
		mrows.Close()
		return LeftNotice{}, err
	}
	mrows.Close()
	_ = now
	return notice, tx.Commit()
}

func (s *SQL) ClearProfile(ctx context.Context, userID int64, username string) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT photo_id FROM user_profile_photos WHERE user_id=? AND deleted=0`, userID)
	if err != nil && !missing(err) {
		return nil, err
	}
	var photos []int64
	if err == nil {
		for rows.Next() {
			var id int64
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			photos = append(photos, id)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	if _, err = s.DB.ExecContext(ctx, `UPDATE user_profile_photos SET deleted=1, date2=0 WHERE user_id=? AND deleted=0`, userID); err != nil && !missing(err) {
		return nil, err
	}
	_, err = s.DB.ExecContext(ctx, `
UPDATE users SET first_name='', last_name='', username='', about='', birthday='', photo_id=0 WHERE id=?`, userID)
	if missingColumn(err) {
		_, err = s.DB.ExecContext(ctx, `
UPDATE users SET first_name='', last_name='', username='', about='', photo_id=0 WHERE id=?`, userID)
	}
	if err != nil {
		return nil, err
	}
	if _, err = s.DB.ExecContext(ctx, `DELETE FROM username WHERE peer_id=? AND peer_type=?`, userID, mtproto.PEER_USER); err != nil && !missing(err) {
		return nil, err
	}
	if username != "" {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM username WHERE username=?`, username)
	}
	return photos, nil
}

func (s *SQL) DeleteContacts(ctx context.Context, userID int64) ([]int64, error) {
	seen := map[int64]struct{}{}
	var owners []int64
	for {
		rows, err := s.DB.QueryContext(ctx, `
SELECT owner_user_id FROM user_contacts WHERE contact_user_id=? AND is_deleted=0 LIMIT 500`, userID)
		if err != nil {
			return owners, err
		}
		var ids []int64
		for rows.Next() {
			var id int64
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return owners, err
			}
			ids = append(ids, id)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return owners, err
		}
		rows.Close()
		if len(ids) == 0 {
			break
		}
		for _, id := range ids {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				owners = append(owners, id)
			}
		}
		if err = s.markContacts(ctx, `contact_user_id=? AND owner_user_id IN (%s)`, userID, ids); err != nil {
			return owners, err
		}
	}
	for {
		res, err := s.execContact(ctx, `
UPDATE user_contacts SET is_deleted=1, mutual=0, close_friend=0, stories_hidden=0
WHERE owner_user_id=? AND is_deleted=0 LIMIT 500`, `
UPDATE user_contacts SET is_deleted=1, mutual=0
WHERE owner_user_id=? AND is_deleted=0 LIMIT 500`, userID)
		if err != nil {
			return owners, err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			break
		}
	}
	return owners, nil
}

func (s *SQL) markContacts(ctx context.Context, where string, userID int64, ids []int64) error {
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	args = append(args, userID)
	for _, id := range ids {
		args = append(args, id)
	}
	full := `UPDATE user_contacts SET is_deleted=1, mutual=0, close_friend=0, stories_hidden=0 WHERE ` + fmt.Sprintf(where, ph)
	plain := `UPDATE user_contacts SET is_deleted=1, mutual=0 WHERE ` + fmt.Sprintf(where, ph)
	_, err := s.DB.ExecContext(ctx, full, args...)
	if missingColumn(err) {
		_, err = s.DB.ExecContext(ctx, plain, args...)
	}
	return err
}

func (s *SQL) execContact(ctx context.Context, full, plain string, args ...any) (sql.Result, error) {
	res, err := s.DB.ExecContext(ctx, full, args...)
	if missingColumn(err) {
		return s.DB.ExecContext(ctx, plain, args...)
	}
	return res, err
}

func (s *SQL) DeleteSettings(ctx context.Context, userID int64) error {
	stmts := []string{
		`DELETE FROM user_privacies WHERE user_id=?`,
		`DELETE FROM user_global_privacy_settings WHERE user_id=?`,
		`UPDATE user_notify_settings SET deleted=1 WHERE user_id=? AND deleted=0`,
		`DELETE FROM user_settings WHERE user_id=?`,
		`DELETE FROM user_peer_settings WHERE user_id=?`,
		`UPDATE user_peer_blocks SET deleted=1 WHERE user_id=? AND deleted=0`,
		`DELETE FROM user_passwords WHERE user_id=?`,
		`DELETE FROM user_password_srp WHERE user_id=?`,
		`DELETE FROM user_password_login WHERE user_id=?`,
		`DELETE FROM devices WHERE user_id=?`,
		`UPDATE dialog_filters SET deleted=1 WHERE user_id=? AND deleted=0`,
		`UPDATE imported_contacts SET deleted=1 WHERE user_id=? AND deleted=0`,
	}
	for _, q := range stmts {
		if _, err := s.DB.ExecContext(ctx, q, userID); err != nil && !missing(err) && !missingColumn(err) {
			return err
		}
	}
	return nil
}

func (s *SQL) PhotoObjects(ctx context.Context, photoID int64, avatar bool) ([]Object, error) {
	paths, err := s.photoPaths(ctx, photoID)
	if err != nil {
		return nil, err
	}
	var out []Object
	for _, path := range paths {
		out = append(out, Object{Kind: "photo", ID: photoID, Class: "photos", Path: path})
		if avatar {
			out = append(out, Object{Kind: "avatar", ID: photoID, Class: "avatars", Path: path})
		}
	}
	return out, nil
}

func (s *SQL) photoPaths(ctx context.Context, photoID int64) ([]string, error) {
	var sizeID int64
	err := s.DB.QueryRowContext(ctx, `SELECT IF(size_id<>0, size_id, photo_id) FROM photos WHERE photo_id=?`, photoID).Scan(&sizeID)
	if err == sql.ErrNoRows {
		sizeID = photoID
	} else if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT file_path FROM photo_sizes WHERE photo_size_id=? AND file_path<>''`, sizeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var path string
		if err = rows.Scan(&path); err != nil {
			return nil, err
		}
		out = append(out, path)
	}
	return out, rows.Err()
}

func (s *SQL) DocumentObjects(ctx context.Context, documentID int64) ([]Object, error) {
	var path string
	err := s.DB.QueryRowContext(ctx, `SELECT file_path FROM documents WHERE document_id=?`, documentID).Scan(&path)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if path == "" {
		path = fmt.Sprintf("%d.dat", documentID)
	}
	return []Object{{Kind: "document", ID: documentID, Class: "documents", Path: path}}, nil
}

func (s *SQL) EncryptedObjects(ctx context.Context, fileID int64) ([]Object, error) {
	var path string
	err := s.DB.QueryRowContext(ctx, `SELECT file_path FROM encrypted_files WHERE encrypted_file_id=?`, fileID).Scan(&path)
	if err == sql.ErrNoRows || missing(err) {
		path = fmt.Sprintf("%d.dat", fileID)
		return []Object{{Kind: "encrypted", ID: fileID, Class: "encryptedfiles", Path: path}}, nil
	}
	if err != nil {
		return nil, err
	}
	if path == "" {
		path = fmt.Sprintf("%d.dat", fileID)
	}
	return []Object{{Kind: "encrypted", ID: fileID, Class: "encryptedfiles", Path: path}}, nil
}

func (s *SQL) Enqueue(ctx context.Context, userID int64, objs []Object) error {
	for _, obj := range objs {
		if obj.Path == "" || obj.Class == "" {
			continue
		}
		_, err := s.DB.ExecContext(ctx, `
INSERT IGNORE INTO account_media_gc (user_id, kind, media_id, class, object_path, status)
VALUES (?,?,?,?,?,'pending')`, userID, obj.Kind, obj.ID, obj.Class, obj.Path)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SQL) Due(ctx context.Context, status string, limit int) ([]GCItem, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT id, user_id, kind, media_id, class, object_path, status, trashed_at
FROM account_media_gc WHERE status=? ORDER BY id LIMIT ?`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GCItem
	for rows.Next() {
		var item GCItem
		if err = rows.Scan(&item.ID, &item.UserID, &item.Kind, &item.MediaID, &item.Class, &item.Path, &item.Status, &item.TrashedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// PrepareChannelMedia loads live channel message media once per GC pass.
// A failure makes Refs refuse to trash anything, so a scan error cannot
// delete a file a channel still shows.
func (s *SQL) PrepareChannelMedia(ctx context.Context) error {
	s.channelPhotos = map[int64]struct{}{}
	s.channelDocs = map[int64]struct{}{}
	s.channelReady = false
	var lastCh int64
	var lastMsg int32
	for {
		rows, err := s.DB.QueryContext(ctx, `
SELECT channel_id, message_id, message_data FROM channel_messages
WHERE deleted=0 AND (channel_id>? OR (channel_id=? AND message_id>?))
ORDER BY channel_id, message_id LIMIT 500`, lastCh, lastCh, lastMsg)
		if err != nil {
			if missing(err) {
				s.channelReady = true
				s.channelErr = false
				return nil
			}
			s.channelErr = true
			return err
		}
		n := 0
		for rows.Next() {
			var data []byte
			if err = rows.Scan(&lastCh, &lastMsg, &data); err != nil {
				rows.Close()
				s.channelErr = true
				return err
			}
			n++
			msg := &mtproto.Message{}
			if proto.Unmarshal(data, msg) != nil || msg.GetMedia() == nil {
				continue
			}
			if d := msg.GetMedia().GetDocument(); d != nil && d.GetId() != 0 {
				s.channelDocs[d.GetId()] = struct{}{}
			}
			if p := msg.GetMedia().GetPhoto_FLAGPHOTO(); p != nil && p.GetId() != 0 {
				s.channelPhotos[p.GetId()] = struct{}{}
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			s.channelErr = true
			return err
		}
		if n < 500 {
			s.channelReady = true
			s.channelErr = false
			return nil
		}
	}
}

func (s *SQL) Refs(ctx context.Context, item GCItem) (int, error) {
	if !s.channelReady || s.channelErr {
		return -1, nil
	}
	switch item.Kind {
	case "photo", "avatar":
		if _, ok := s.channelPhotos[item.MediaID]; ok {
			return 1, nil
		}
		n, err := s.profileRefs(ctx, item.MediaID)
		if err != nil || n > 0 {
			return n, err
		}
		return s.messageRefs(ctx, exprPhotoID, item.MediaID)
	case "document":
		if _, ok := s.channelDocs[item.MediaID]; ok {
			return 1, nil
		}
		if n, err := s.stickerRefs(ctx, item.MediaID); err != nil || n > 0 {
			return n, err
		}
		return s.messageRefs(ctx, exprDocumentID, item.MediaID)
	case "encrypted":
		return 0, nil
	default:
		return -1, nil
	}
}

func (s *SQL) profileRefs(ctx context.Context, photoID int64) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM user_profile_photos WHERE photo_id=? AND deleted=0 LIMIT 1`, photoID).Scan(&n)
	if err == sql.ErrNoRows || missing(err) {
		return 0, nil
	}
	return n, err
}

func (s *SQL) stickerRefs(ctx context.Context, documentID int64) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM tellomi_sticker_items WHERE document_id=? LIMIT 1`, documentID).Scan(&n)
	if err == sql.ErrNoRows || missing(err) || missingColumn(err) {
		return 0, nil
	}
	return n, err
}

func (s *SQL) messageRefs(ctx context.Context, expr string, id int64) (int, error) {
	for _, table := range s.allTables() {
		var n int
		q := `SELECT 1 FROM ` + table + ` WHERE deleted=0 AND ` + expr + `=? LIMIT 1`
		err := s.DB.QueryRowContext(ctx, q, id).Scan(&n)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return -1, nil
		}
		if n > 0 {
			return 1, nil
		}
	}
	return 0, nil
}

func blobKind(item GCItem) string {
	switch item.Kind {
	case "document":
		return mediablob.KindDocument
	case "photo", "avatar":
		return mediablob.KindPhoto
	default:
		return ""
	}
}

// Shared reports a different media_blob_refs row for this object key.
// A missing dedup table means dedup is off, so nothing else holds the key.
func (s *SQL) Shared(ctx context.Context, item GCItem) (bool, error) {
	kind := blobKind(item)
	if kind == "" || s == nil || s.DB == nil {
		return false, nil
	}
	var n int
	// Another row that shares this media's blob, even when the queued key is
	// a derivative path rather than the blob's own file_path.
	err := s.DB.QueryRowContext(ctx, `
SELECT 1 FROM media_blob_refs mine
JOIN media_blob_refs other ON other.blob_id = mine.blob_id
WHERE mine.ref_kind = ? AND mine.ref_id = ?
  AND NOT (other.ref_kind = ? AND other.ref_id = ?)
LIMIT 1`, kind, item.MediaID, kind, item.MediaID).Scan(&n)
	if err == nil {
		return true, nil
	}
	if err != sql.ErrNoRows && !missing(err) && !missingColumn(err) {
		return false, err
	}
	err = s.DB.QueryRowContext(ctx, `
SELECT 1 FROM media_blobs b
JOIN media_blob_refs r ON r.blob_id = b.id
WHERE b.storage_class = ? AND b.file_path = ?
  AND NOT (r.ref_kind = ? AND r.ref_id = ?)
LIMIT 1`, item.Class, item.Path, kind, item.MediaID).Scan(&n)
	if err == nil {
		return true, nil
	}
	if err != sql.ErrNoRows && !missing(err) && !missingColumn(err) {
		return false, err
	}
	err = s.DB.QueryRowContext(ctx, `
SELECT 1 FROM media_photo_paths p
JOIN media_blob_refs r ON r.ref_kind = ? AND r.ref_id = p.photo_id
WHERE p.file_path = ? AND NOT (r.ref_kind = ? AND r.ref_id = ?)
LIMIT 1`, mediablob.KindPhoto, item.Path, kind, item.MediaID).Scan(&n)
	if err == sql.ErrNoRows || missing(err) || missingColumn(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Detach releases this item's blob ref before the object is removed.
// remove is false when another ref still holds the object.
func (s *SQL) Detach(ctx context.Context, item GCItem) (bool, error) {
	kind := blobKind(item)
	if kind == "" || s == nil || s.DB == nil {
		return true, nil
	}
	rel, err := mediablob.New(s.DB).Release(ctx, kind, item.MediaID)
	if err != nil {
		return false, err
	}
	if !rel.Found {
		return true, nil
	}
	return rel.Remove, nil
}

func (s *SQL) Exempt(ctx context.Context, item GCItem) (bool, error) {
	if item.Kind != "document" {
		return false, nil
	}
	var mime, attrs string
	err := s.DB.QueryRowContext(ctx, `SELECT mime_type, CAST(attributes AS CHAR) FROM documents WHERE document_id=?`, item.MediaID).Scan(&mime, &attrs)
	if err == sql.ErrNoRows || missing(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return DocumentExempt(mime, attrs), nil
}

func (s *SQL) Mark(ctx context.Context, id int64, status string, trashedAt int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE account_media_gc SET status=?, trashed_at=? WHERE id=?`, status, trashedAt, id)
	return err
}

func missing(err error) bool {
	var me *mysqldriver.MySQLError
	return errors.As(err, &me) && me.Number == 1146
}

func missingColumn(err error) bool {
	var me *mysqldriver.MySQLError
	return errors.As(err, &me) && me.Number == 1054
}
