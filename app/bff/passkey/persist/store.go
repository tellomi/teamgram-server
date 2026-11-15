package persist

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/teamgram/teamgram-server/pkg/mysqlpool"
	"github.com/teamgram/teamgram-server/pkg/mysqltls"
)

const queryTimeout = 5 * time.Second

// Saved is one passkey row shown by account.getPasskeys.
type Saved struct {
	ID       string
	Name     string
	Date     int32
	LastUsed int32
}

// Service reads and writes passkeys in MySQL. It keeps no credential cache.
type Service struct {
	db *sql.DB
}

var (
	mu  sync.RWMutex
	cur *Service
)

// Set installs the process-wide store. Tests use it for a second connection.
func Set(s *Service) {
	mu.Lock()
	cur = s
	mu.Unlock()
}

// Current is the process-wide store, or nil before Configure.
func Current() *Service {
	mu.RLock()
	defer mu.RUnlock()
	return cur
}

// OpenDB uses an already open pool. The caller applied the passkey migration.
func OpenDB(db *sql.DB) *Service {
	if db == nil {
		return nil
	}
	return &Service{db: db}
}

// Configure opens TEAMGRAM_MYSQL_DSN or dsn and installs the store.
// An empty DSN or a failed ping leaves the store unset.
func Configure(dsn string) error {
	if env := strings.TrimSpace(os.Getenv("TEAMGRAM_MYSQL_DSN")); env != "" {
		dsn = env
	}
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return errors.New("passkey mysql dsn is empty")
	}
	db, err := mysqltls.Open(dsn)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = mysqlpool.Discard(db)
		return err
	}
	Set(OpenDB(db))
	return nil
}

// ResetForTest deletes passkey rows in the current store. Production does not call it.
func ResetForTest() {
	s := Current()
	if s == nil || s.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	_, _ = s.db.ExecContext(ctx, "DELETE FROM user_passkeys")
	_, _ = s.db.ExecContext(ctx, "DELETE FROM user_passkey_challenges")
}

func (s *Service) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), queryTimeout)
}

type credentialRow struct {
	ID              string
	UserID          int64
	RawID           []byte
	PublicKey       []byte
	SignCount       uint32
	AAGUID          []byte
	AttestationType string
	UserHandle      string
	UserPresent     bool
	UserVerified    bool
	BackupEligible  bool
	BackupState     bool
	Name            string
	CreatedAt       int32
	LastUsed        int32
}

func (s *Service) saveChallenge(userID, authKey int64, kind string, session *sessionBlob) error {
	ctx, cancel := s.ctx()
	defer cancel()
	now := int32(time.Now().Unix())
	_, _ = s.db.ExecContext(ctx, "DELETE FROM user_passkey_challenges WHERE expires_at < ?", now)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO user_passkey_challenges (challenge, user_id, auth_key_id, kind, session_json, expires_at)
VALUES (?, ?, ?, ?, ?, ?)`, session.Data.Challenge, userID, authKey, kind, session.raw, session.expires)
	return err
}

func (s *Service) takeChallenge(challenge, kind string, userID, authKey int64, matchUser bool) (*sessionBlob, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	now := int32(time.Now().Unix())
	var (
		raw   string
		owner int64
		keyID int64
		got   string
		exp   int32
	)
	err := s.db.QueryRowContext(ctx, `
SELECT user_id, auth_key_id, kind, session_json, expires_at
FROM user_passkey_challenges WHERE challenge = ?`, challenge).Scan(&owner, &keyID, &got, &raw, &exp)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if errors.Is(err, sql.ErrNoRows) || exp < now || got != kind || (matchUser && owner != userID) {
		return nil, errInvalid
	}
	// A different permanent key must not consume the ceremony. The key that
	// called init can still finish.
	if authKey == 0 || keyID != authKey {
		return nil, errInvalid
	}
	res, err := s.db.ExecContext(ctx, "DELETE FROM user_passkey_challenges WHERE challenge = ? AND auth_key_id = ?", challenge, authKey)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, errInvalid
	}
	return decodeSession(raw)
}

func (s *Service) byRaw(raw []byte) (credentialRow, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	var row credentialRow
	var present, verified, eligible, state int
	err := s.db.QueryRowContext(ctx, `
SELECT credential_id, user_id, raw_id, public_key, sign_count, aaguid, attestation_type,
       user_handle, user_present, user_verified, backup_eligible, backup_state, name, created_at, last_used_at
FROM user_passkeys WHERE raw_id = ?`, raw).Scan(
		&row.ID, &row.UserID, &row.RawID, &row.PublicKey, &row.SignCount, &row.AAGUID, &row.AttestationType,
		&row.UserHandle, &present, &verified, &eligible, &state, &row.Name, &row.CreatedAt, &row.LastUsed,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return credentialRow{}, errNotFound
	}
	if err != nil {
		return credentialRow{}, err
	}
	row.UserPresent = present != 0
	row.UserVerified = verified != 0
	row.BackupEligible = eligible != 0
	row.BackupState = state != 0
	return row, nil
}

func (s *Service) list(userID int64) ([]Saved, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `
SELECT credential_id, name, created_at, last_used_at
FROM user_passkeys WHERE user_id = ? ORDER BY created_at, credential_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Saved, 0)
	for rows.Next() {
		var row Saved
		if err = rows.Scan(&row.ID, &row.Name, &row.Date, &row.LastUsed); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Service) rawIDs(userID int64) ([][]byte, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `SELECT raw_id FROM user_passkeys WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, rows.Err()
}

func (s *Service) insert(row credentialRow) error {
	ctx, cancel := s.ctx()
	defer cancel()
	var owner int64
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM user_passkeys WHERE raw_id = ?`, row.RawID).Scan(&owner)
	if err == nil {
		return errInvalid
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	bit := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO user_passkeys
(raw_id, credential_id, user_id, public_key, sign_count, aaguid, attestation_type, user_handle,
 user_present, user_verified, backup_eligible, backup_state, name, created_at, last_used_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		row.RawID, row.ID, row.UserID, row.PublicKey, row.SignCount, row.AAGUID, row.AttestationType, row.UserHandle,
		bit(row.UserPresent), bit(row.UserVerified), bit(row.BackupEligible), bit(row.BackupState),
		row.Name, row.CreatedAt)
	return err
}

func (s *Service) saveCounter(raw []byte, old, next uint32, row credentialRow, used int32) error {
	ctx, cancel := s.ctx()
	defer cancel()
	bit := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE user_passkeys
SET sign_count=?, last_used_at=?, user_present=?, user_verified=?, backup_eligible=?, backup_state=?
WHERE raw_id=? AND sign_count=?`,
		next, used, bit(row.UserPresent), bit(row.UserVerified), bit(row.BackupEligible), bit(row.BackupState), raw, old)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errInvalid
	}
	return nil
}

func (s *Service) delete(userID int64, raw []byte) (bool, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	res, err := s.db.ExecContext(ctx, `DELETE FROM user_passkeys WHERE user_id=? AND raw_id=?`, userID, raw)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}
