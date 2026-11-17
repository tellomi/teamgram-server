package dao

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/teamgram/teamgram-server/pkg/mysqltls"
)

// EncryptedFileRow is one encrypted_files record. upload.getFile authorizes
// inputEncryptedFileLocation by this access_hash. expired_at stays 0: the
// retention sweeper treats 0 as still stored and does not delete secret-chat
// files (docs/media-retention.md, migrate-20261103-media-retention.sql).
type EncryptedFileRow struct {
	ID             int64
	AccessHash     int64
	DCID           int32
	Size           int64
	KeyFingerprint int32
	MD5            string
	Path           string
}

// EncryptedFiles writes encrypted_files. A nil store means the DSN is unset.
type EncryptedFiles struct {
	db *sql.DB
}

// OpenEncryptedFiles opens the teamgram pool. An empty DSN returns nil.
// The pool is shared with other callers of the same DSN and is not closed.
func OpenEncryptedFiles(dsn string) (*EncryptedFiles, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, nil
	}
	db, err := mysqltls.Open(dsn)
	if err != nil {
		return nil, err
	}
	return NewEncryptedFiles(db), nil
}

// NewEncryptedFiles wraps a pool. Tests pass their own *sql.DB.
func NewEncryptedFiles(db *sql.DB) *EncryptedFiles {
	if db == nil {
		return nil
	}
	return &EncryptedFiles{db: db}
}

// SaveEncryptedFile writes the encrypted_files row for one upload.
func (d *Dao) SaveEncryptedFile(ctx context.Context, row EncryptedFileRow) (EncryptedFileRow, error) {
	if d == nil || d.Encrypted == nil {
		return EncryptedFileRow{}, errors.New("dfs: encrypted file index is not configured")
	}
	return d.Encrypted.Save(ctx, row)
}

// Save inserts row once. A second save of the same encrypted_file_id returns
// the stored row and does not insert another. The first access_hash, size,
// and key_fingerprint stay, so a retry still matches upload.getFile.
func (s *EncryptedFiles) Save(ctx context.Context, row EncryptedFileRow) (EncryptedFileRow, error) {
	if s == nil || s.db == nil || row.ID == 0 {
		return EncryptedFileRow{}, errors.New("dfs: encrypted file index is not configured")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return EncryptedFileRow{}, err
	}
	defer func() { _ = tx.Rollback() }()

	stored, found, err := loadEncryptedFile(ctx, tx, row.ID)
	if err != nil {
		return EncryptedFileRow{}, err
	}
	if found {
		if err = tx.Commit(); err != nil {
			return EncryptedFileRow{}, err
		}
		return stored, nil
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO encrypted_files
  (encrypted_file_id, access_hash, dc_id, file_size, key_fingerprint, md5_checksum, file_path, expired_at)
VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
		row.ID, row.AccessHash, row.DCID, row.Size, row.KeyFingerprint, row.MD5, row.Path)
	if err != nil {
		if !dupEncryptedFile(err) {
			return EncryptedFileRow{}, err
		}
		stored, found, err = loadEncryptedFile(ctx, tx, row.ID)
		if err != nil {
			return EncryptedFileRow{}, err
		}
		if !found {
			return EncryptedFileRow{}, errors.New("dfs: encrypted file row missing after duplicate insert")
		}
		if err = tx.Commit(); err != nil {
			return EncryptedFileRow{}, err
		}
		return stored, nil
	}
	if err = tx.Commit(); err != nil {
		return EncryptedFileRow{}, err
	}
	return row, nil
}

type rowQuery interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func loadEncryptedFile(ctx context.Context, q rowQuery, id int64) (EncryptedFileRow, bool, error) {
	var row EncryptedFileRow
	err := q.QueryRowContext(ctx, `
SELECT access_hash, dc_id, file_size, key_fingerprint, md5_checksum, file_path
FROM encrypted_files
WHERE encrypted_file_id = ?
LIMIT 1
FOR UPDATE`, id).Scan(&row.AccessHash, &row.DCID, &row.Size, &row.KeyFingerprint, &row.MD5, &row.Path)
	if errors.Is(err, sql.ErrNoRows) {
		return EncryptedFileRow{}, false, nil
	}
	if err != nil {
		return EncryptedFileRow{}, false, err
	}
	row.ID = id
	return row, true, nil
}

func dupEncryptedFile(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}
