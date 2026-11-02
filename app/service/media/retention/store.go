package retention

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
)

// The JSON paths of message media in messages.message_data. The migration
// creates functional indexes on exactly these expressions; MySQL only uses
// an index when the query repeats the expression verbatim.
const (
	exprDocumentID = "JSON_VALUE(message_data, '$.media.document.id' RETURNING SIGNED NULL ON EMPTY NULL ON ERROR)"
	exprPhotoID    = "JSON_VALUE(message_data, '$.media.photo_FLAGPHOTO.id' RETURNING SIGNED NULL ON EMPTY NULL ON ERROR)"
)

// Peer types in messages.peer_type.
const (
	peerUser    = 2
	peerChat    = 3
	peerChannel = 4
)

// Holder is one message box that references a media item.
type Holder struct {
	UserID   int64
	PeerType int32
	PeerID   int64
}

// Store is the database side of the sweeper.
type Store interface {
	ListCandidates(ctx context.Context, kind Kind, cutoff, afterID int64, limit int) ([]Media, error)
	MessageHolders(ctx context.Context, kind Kind, id int64) ([]Holder, error)
	ChatMembers(ctx context.Context, chatID int64) ([]int64, error)
	ChannelMembers(ctx context.Context, channelID int64) ([]int64, error)
	// ChannelMedia maps media id -> channel ids for every live channel message.
	ChannelMedia(ctx context.Context) (map[Kind]map[int64][]int64, error)
	Subscriptions(ctx context.Context, userIDs []int64) ([]Subscription, error)
	MarkExpired(ctx context.Context, kind Kind, id, at int64) error
	ExpiredAt(ctx context.Context, kind Kind, id int64) (int64, error)
	CountOld(ctx context.Context, kind Kind, cutoff int64) (int64, error)
}

// SQLStore implements Store on the teamgram MySQL schema.
type SQLStore struct {
	DB *sql.DB
}

func OpenSQL(dsn string) (*SQLStore, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	return &SQLStore{DB: db}, nil
}

func table(kind Kind) (tbl, idCol string, err error) {
	switch kind {
	case KindDocument:
		return "documents", "document_id", nil
	case KindPhoto:
		return "photos", "photo_id", nil
	case KindEncrypted:
		return "encrypted_files", "encrypted_file_id", nil
	}
	return "", "", fmt.Errorf("unknown media kind %q", kind)
}

// uploadedExpr is the original upload time: date2 when set, else created_at.
func uploadedExpr(kind Kind) string {
	if kind == KindEncrypted {
		return "UNIX_TIMESTAMP(created_at)"
	}
	return "IF(date2 > 0, date2, UNIX_TIMESTAMP(created_at))"
}

func (s *SQLStore) ListCandidates(ctx context.Context, kind Kind, cutoff, afterID int64, limit int) ([]Media, error) {
	var q string
	switch kind {
	case KindDocument:
		q = `SELECT document_id, CAST(` + uploadedExpr(kind) + ` AS SIGNED), file_size, mime_type, CAST(attributes AS CHAR), ''
FROM documents WHERE expired_at = 0 AND document_id > ? AND ` + uploadedExpr(kind) + ` < ?
ORDER BY document_id LIMIT ?`
	case KindPhoto:
		q = `SELECT p.photo_id, CAST(IF(p.date2 > 0, p.date2, UNIX_TIMESTAMP(p.created_at)) AS SIGNED),
  CAST(COALESCE((SELECT SUM(ps.file_size) FROM photo_sizes ps WHERE ps.photo_size_id = IF(p.size_id <> 0, p.size_id, p.photo_id) AND ps.size_type IN ('m','x','y','w')), 0) AS SIGNED),
  '', '',
  COALESCE((SELECT GROUP_CONCAT(ps.size_type) FROM photo_sizes ps WHERE ps.photo_size_id = IF(p.size_id <> 0, p.size_id, p.photo_id)), '')
FROM photos p WHERE p.expired_at = 0 AND p.photo_id > ? AND IF(p.date2 > 0, p.date2, UNIX_TIMESTAMP(p.created_at)) < ?
ORDER BY p.photo_id LIMIT ?`
	default:
		return nil, fmt.Errorf("listing %s media is not supported", kind)
	}
	rows, err := s.DB.QueryContext(ctx, q, afterID, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("list %s candidates: %w", kind, err)
	}
	defer rows.Close()
	var out []Media
	for rows.Next() {
		m := Media{Kind: kind}
		var sizes string
		var attrs sql.NullString
		if err := rows.Scan(&m.ID, &m.UploadedAt, &m.Size, &m.MimeType, &attrs, &sizes); err != nil {
			return nil, err
		}
		m.Attributes = attrs.String
		if sizes != "" {
			m.SizeTypes = strings.Split(sizes, ",")
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *SQLStore) MessageHolders(ctx context.Context, kind Kind, id int64) ([]Holder, error) {
	var expr string
	switch kind {
	case KindDocument:
		expr = exprDocumentID
	case KindPhoto:
		expr = exprPhotoID
	default:
		return nil, nil
	}
	// IN (?) matches the index expression; MySQL picks a range scan.
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT user_id, peer_type, peer_id FROM messages WHERE `+expr+` IN (?) AND deleted = 0`, id)
	if err != nil {
		return nil, fmt.Errorf("message holders of %s %d: %w", kind, id, err)
	}
	defer rows.Close()
	var out []Holder
	for rows.Next() {
		var h Holder
		if err := rows.Scan(&h.UserID, &h.PeerType, &h.PeerID); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *SQLStore) ids(ctx context.Context, q string, args ...any) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *SQLStore) ChatMembers(ctx context.Context, chatID int64) ([]int64, error) {
	return s.ids(ctx, `SELECT user_id FROM chat_participants WHERE chat_id = ? AND state = 0`, chatID)
}

func (s *SQLStore) ChannelMembers(ctx context.Context, channelID int64) ([]int64, error) {
	return s.ids(ctx, `SELECT user_id FROM channel_participants WHERE channel_id = ? AND state = 0`, channelID)
}

// ChannelMedia decodes every live channel message once. channel_messages
// stores protobuf, so the media id cannot be indexed in SQL.
func (s *SQLStore) ChannelMedia(ctx context.Context) (map[Kind]map[int64][]int64, error) {
	out := map[Kind]map[int64][]int64{KindDocument: {}, KindPhoto: {}}
	var lastCh int64
	var lastMsg int32
	for {
		rows, err := s.DB.QueryContext(ctx, `SELECT channel_id, message_id, message_data FROM channel_messages
WHERE deleted = 0 AND (channel_id > ? OR (channel_id = ? AND message_id > ?))
ORDER BY channel_id, message_id LIMIT 1000`, lastCh, lastCh, lastMsg)
		if err != nil {
			return nil, fmt.Errorf("scan channel_messages: %w", err)
		}
		n := 0
		for rows.Next() {
			var data []byte
			if err := rows.Scan(&lastCh, &lastMsg, &data); err != nil {
				rows.Close()
				return nil, err
			}
			n++
			kind, id := channelMediaID(data)
			if id != 0 {
				out[kind][id] = appendUnique(out[kind][id], lastCh)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		if n < 1000 {
			return out, nil
		}
	}
}

func channelMediaID(data []byte) (Kind, int64) {
	msg := &mtproto.Message{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return "", 0
	}
	media := msg.GetMedia()
	if media == nil {
		return "", 0
	}
	if d := media.GetDocument(); d != nil && d.GetId() != 0 {
		return KindDocument, d.GetId()
	}
	if p := media.GetPhoto_FLAGPHOTO(); p != nil && p.GetId() != 0 {
		return KindPhoto, p.GetId()
	}
	return "", 0
}

func appendUnique(xs []int64, v int64) []int64 {
	for _, x := range xs {
		if x == v {
			return xs
		}
	}
	return append(xs, v)
}

func (s *SQLStore) Subscriptions(ctx context.Context, userIDs []int64) ([]Subscription, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(userIDs)), ",")
	args := make([]any, len(userIDs))
	for i, id := range userIDs {
		args[i] = id
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, premium <> 0, CAST(COALESCE(premium_expire_date, 0) AS SIGNED) FROM users WHERE id IN (`+ph+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("subscriptions: %w", err)
	}
	defer rows.Close()
	var out []Subscription
	for rows.Next() {
		var sub Subscription
		if err := rows.Scan(&sub.UserID, &sub.Premium, &sub.ExpireDate); err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

func (s *SQLStore) MarkExpired(ctx context.Context, kind Kind, id, at int64) error {
	tbl, col, err := table(kind)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE `+tbl+` SET expired_at = ? WHERE `+col+` = ? AND expired_at = 0`, at, id)
	return err
}

func (s *SQLStore) ExpiredAt(ctx context.Context, kind Kind, id int64) (int64, error) {
	tbl, col, err := table(kind)
	if err != nil {
		return 0, err
	}
	var at int64
	err = s.DB.QueryRowContext(ctx, `SELECT expired_at FROM `+tbl+` WHERE `+col+` = ? LIMIT 1`, id).Scan(&at)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return at, err
}

func (s *SQLStore) CountOld(ctx context.Context, kind Kind, cutoff int64) (int64, error) {
	tbl, _, err := table(kind)
	if err != nil {
		return 0, err
	}
	var n int64
	err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+tbl+` WHERE expired_at = 0 AND `+uploadedExpr(kind)+` < ?`, cutoff).Scan(&n)
	return n, err
}
