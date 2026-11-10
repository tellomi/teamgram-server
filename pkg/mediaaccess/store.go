package mediaaccess

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/teamgram/teamgram-server/pkg/mysqltls"

	"github.com/go-sql-driver/mysql"
)

// Store reads the teamgram tables that say whether a caller may use a
// file. A nil Store means the DSN is unset; callers fail closed.
type Store struct {
	db *sql.DB
}

// Open opens a small pool. An empty DSN returns a nil Store.
func Open(dsn string) (*Store, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, nil
	}
	db, err := mysqltls.Open(dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	return &Store{db: db}, nil
}

// EncryptedAccess is the access_hash stored for an encrypted file.
func (s *Store) EncryptedAccess(ctx context.Context, id int64) (int64, bool, error) {
	if s == nil || s.db == nil || id == 0 {
		return 0, false, nil
	}
	var hash int64
	err := s.db.QueryRowContext(ctx, `SELECT access_hash FROM encrypted_files WHERE encrypted_file_id = ? LIMIT 1`, id).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return hash, true, nil
}

// UserSeesDocument reports whether userID already has this document: a
// live message in their box (including a forward) or a saved GIF. Channel
// posts live in channel_messages as protobuf and are not scanned here;
// members download those with the access_hash on the post.
func (s *Store) UserSeesDocument(ctx context.Context, userID, docID int64) (bool, error) {
	if s == nil || s.db == nil || userID == 0 || docID == 0 {
		return false, nil
	}
	var one int
	err := s.db.QueryRowContext(ctx, `
SELECT 1 FROM messages
WHERE user_id = ? AND deleted = 0
AND media_document_id = ?
LIMIT 1`, userID, docID).Scan(&one)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT 1 FROM tellomi_saved_gifs WHERE user_id = ? AND document_id = ? LIMIT 1`, userID, docID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) || missingTable(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ChatPhoto is the group's current photo when userID is still a member.
func (s *Store) ChatPhoto(ctx context.Context, userID, chatID int64) (photoID int64, member bool, err error) {
	if s == nil || s.db == nil || userID == 0 || chatID == 0 {
		return 0, false, nil
	}
	err = s.db.QueryRowContext(ctx, `
SELECT c.photo_id
FROM chats c
INNER JOIN chat_participants p ON p.chat_id = c.id AND p.user_id = ? AND p.state = 0 AND p.kicked_at = 0 AND p.left_at = 0
WHERE c.id = ?
LIMIT 1`, userID, chatID).Scan(&photoID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return photoID, true, nil
}

// ChannelPhoto is the channel's current photo and access_hash.
func (s *Store) ChannelPhoto(ctx context.Context, channelID int64) (photoID, access int64, found bool, err error) {
	if s == nil || s.db == nil || channelID == 0 {
		return 0, 0, false, nil
	}
	err = s.db.QueryRowContext(ctx, `SELECT photo_id, access_hash FROM channels WHERE id = ? AND deleted = 0 LIMIT 1`, channelID).Scan(&photoID, &access)
	if errors.Is(err, sql.ErrNoRows) || missingTable(err) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	return photoID, access, true, nil
}

func missingTable(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && (me.Number == 1146 || me.Number == 1054)
}
