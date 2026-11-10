package password

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/teamgram/teamgram-server/pkg/mysqltls"
	"github.com/zeromicro/go-zero/core/logx"
)

// schemaSQL matches teamgramd/deploy/sql/migrate-20261101-cloud-password.sql.
// Opening the store creates the tables when that migration has not run yet.
const schemaSQL = `
CREATE TABLE IF NOT EXISTS user_passwords (
  user_id BIGINT NOT NULL,
  has_password TINYINT NOT NULL DEFAULT 0,
  algo VARCHAR(96) NOT NULL DEFAULT '',
  salt1 VARBINARY(128) NOT NULL,
  salt2 VARBINARY(64) NOT NULL,
  verifier VARBINARY(256) NOT NULL,
  hint VARCHAR(128) NOT NULL DEFAULT '',
  email VARCHAR(255) NOT NULL DEFAULT '',
  email_unconfirmed VARCHAR(255) NOT NULL DEFAULT '',
  email_code VARCHAR(16) NOT NULL DEFAULT '',
  email_code_expires BIGINT NOT NULL DEFAULT 0,
  email_code_sent_at BIGINT NOT NULL DEFAULT 0,
  email_code_attempts INT NOT NULL DEFAULT 0,
  secure_algo VARCHAR(32) NOT NULL DEFAULT '',
  secure_salt VARBINARY(64) NOT NULL,
  secure_secret VARBINARY(1024) NOT NULL,
  secure_secret_id BIGINT NOT NULL DEFAULT 0,
  pending_reset_date BIGINT NOT NULL DEFAULT 0,
  version BIGINT NOT NULL DEFAULT 0,
  fail_count INT NOT NULL DEFAULT 0,
  flood_until BIGINT NOT NULL DEFAULT 0,
  new_salt1 VARBINARY(64) NOT NULL,
  new_salt2 VARBINARY(64) NOT NULL,
  new_secure_salt VARBINARY(64) NOT NULL,
  password_set_at BIGINT NOT NULL DEFAULT 0,
  updated_at BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS user_password_srp (
  srp_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  auth_key_id BIGINT NOT NULL DEFAULT 0,
  b_priv VARBINARY(256) NOT NULL,
  b_pub VARBINARY(256) NOT NULL,
  version BIGINT NOT NULL,
  expires BIGINT NOT NULL,
  PRIMARY KEY (srp_id),
  KEY user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS user_password_login (
  auth_key_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  PRIMARY KEY (auth_key_id),
  KEY user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
`

type mysqlStore struct {
	db *sql.DB
}

// openDatabase is sql.Open. Tests replace it to simulate a driver failure.
var openDatabase = sql.Open

func openStore(dsn string) Store {
	dsn = strings.TrimSpace(dsn)
	if env := strings.TrimSpace(os.Getenv("TEAMGRAM_MYSQL_DSN")); env != "" {
		dsn = env
	}
	if dsn == "" {
		return newMemoryStore()
	}
	dsn, err := mysqltls.Apply(dsn)
	if err != nil {
		logx.Errorf("cloud password: mysql tls: %v", err)
		return newMemoryStore()
	}
	db, err := openDatabase("mysql", dsn)
	if err != nil {
		logx.Errorf("cloud password: mysql open failed, state stays in memory: %v", err)
		return newMemoryStore()
	}
	db.SetMaxOpenConns(8)
	db.SetConnMaxLifetime(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		logx.Errorf("cloud password: mysql ping failed, state stays in memory: %v", err)
		_ = db.Close()
		return newMemoryStore()
	}
	if err = execSchema(ctx, db); err != nil {
		logx.Errorf("cloud password: schema: %v", err)
	}
	return &mysqlStore{db: db}
}

func execSchema(ctx context.Context, db *sql.DB) error {
	for _, stmt := range strings.Split(schemaSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	// Existing installs created the table before password_set_at existed.
	// Duplicate column is the steady state after the first boot.
	if _, err := db.ExecContext(ctx, `ALTER TABLE user_passwords ADD COLUMN password_set_at BIGINT NOT NULL DEFAULT 0`); err != nil && !dupColumn(err) {
		return err
	}
	return nil
}

func dupColumn(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Duplicate column") || strings.Contains(msg, "1060")
}

const (
	identityEmailSQL = `SELECT email FROM users WHERE id = ? AND deleted = 0`
	identityOwnerSQL = `SELECT id FROM users WHERE email = ? AND deleted = 0 LIMIT 1`
)

func (s *mysqlStore) IdentityEmail(ctx context.Context, userID int64) (string, error) {
	if s == nil || s.db == nil || userID == 0 {
		return "", nil
	}
	var email sql.NullString
	err := s.db.QueryRowContext(ctx, identityEmailSQL, userID).Scan(&email)
	if err == sql.ErrNoRows {
		return "", ErrIdentityUnconfirmed
	}
	if err != nil {
		return "", err
	}
	if !email.Valid || email.String == "" {
		return "", nil
	}
	if strings.TrimSpace(email.String) == "" {
		return "", ErrIdentityUnconfirmed
	}
	return email.String, nil
}

func (s *mysqlStore) IdentityOwner(ctx context.Context, email string) (int64, error) {
	if s == nil || s.db == nil || email == "" {
		return 0, nil
	}
	var id int64
	err := s.db.QueryRowContext(ctx, identityOwnerSQL, email).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s *mysqlStore) Load(ctx context.Context, userID int64) (Record, error) {
	var rec Record
	var has int
	err := s.db.QueryRowContext(ctx, `
SELECT has_password, algo, salt1, salt2, verifier, hint, email, email_unconfirmed,
  email_code, email_code_expires, email_code_sent_at, email_code_attempts,
  secure_algo, secure_salt, secure_secret, secure_secret_id, pending_reset_date,
  version, fail_count, flood_until, new_salt1, new_salt2, new_secure_salt, password_set_at
FROM user_passwords WHERE user_id = ?`, userID).Scan(
		&has, &rec.Algo, &rec.Salt1, &rec.Salt2, &rec.Verifier, &rec.Hint, &rec.Email, &rec.EmailUnconfirmed,
		&rec.EmailCode, &rec.EmailCodeExpires, &rec.EmailCodeSentAt, &rec.EmailCodeAttempts,
		&rec.SecureAlgo, &rec.SecureSalt, &rec.SecureSecret, &rec.SecureSecretID, &rec.PendingReset,
		&rec.Version, &rec.FailCount, &rec.FloodUntil, &rec.NewSalt1, &rec.NewSalt2, &rec.NewSecureSalt,
		&rec.PasswordSetAt,
	)
	if err == sql.ErrNoRows {
		return Record{UserID: userID}, nil
	}
	if err != nil {
		return Record{}, err
	}
	rec.UserID = userID
	rec.HasPassword = has != 0
	rec.Exists = true
	return rec, nil
}

func (s *mysqlStore) Save(ctx context.Context, rec Record) error {
	has := 0
	if rec.HasPassword {
		has = 1
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO user_passwords (
  user_id, has_password, algo, salt1, salt2, verifier, hint, email, email_unconfirmed,
  email_code, email_code_expires, email_code_sent_at, email_code_attempts,
  secure_algo, secure_salt, secure_secret, secure_secret_id, pending_reset_date,
  version, fail_count, flood_until, new_salt1, new_salt2, new_secure_salt, password_set_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  has_password = VALUES(has_password), algo = VALUES(algo), salt1 = VALUES(salt1),
  salt2 = VALUES(salt2), verifier = VALUES(verifier), hint = VALUES(hint),
  email = VALUES(email), email_unconfirmed = VALUES(email_unconfirmed),
  email_code = VALUES(email_code), email_code_expires = VALUES(email_code_expires),
  email_code_sent_at = VALUES(email_code_sent_at), email_code_attempts = VALUES(email_code_attempts),
  secure_algo = VALUES(secure_algo), secure_salt = VALUES(secure_salt),
  secure_secret = VALUES(secure_secret), secure_secret_id = VALUES(secure_secret_id),
  pending_reset_date = VALUES(pending_reset_date), version = VALUES(version),
  fail_count = VALUES(fail_count), flood_until = VALUES(flood_until),
  new_salt1 = VALUES(new_salt1), new_salt2 = VALUES(new_salt2),
  new_secure_salt = VALUES(new_secure_salt), password_set_at = VALUES(password_set_at),
  updated_at = VALUES(updated_at)`,
		rec.UserID, has, rec.Algo, nz(rec.Salt1), nz(rec.Salt2), nz(rec.Verifier), rec.Hint, rec.Email, rec.EmailUnconfirmed,
		rec.EmailCode, rec.EmailCodeExpires, rec.EmailCodeSentAt, rec.EmailCodeAttempts,
		rec.SecureAlgo, nz(rec.SecureSalt), nz(rec.SecureSecret), rec.SecureSecretID, rec.PendingReset,
		rec.Version, rec.FailCount, rec.FloodUntil, nz(rec.NewSalt1), nz(rec.NewSalt2), nz(rec.NewSecureSalt),
		rec.PasswordSetAt, time.Now().Unix(),
	)
	return err
}

func (s *mysqlStore) PutChallenge(ctx context.Context, ch Challenge) error {
	// Indexed by user_id. Expired rows for this user only; a live srp_id
	// stays valid until it expires or the password changes.
	cutoff := ch.Expires - int64(challengeTTL/time.Second)
	_, _ = s.db.ExecContext(ctx, `
DELETE FROM user_password_srp WHERE user_id = ? AND expires <= ?`,
		ch.UserID, cutoff)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO user_password_srp (srp_id, user_id, auth_key_id, b_priv, b_pub, version, expires)
VALUES (?,?,?,?,?,?,?)`,
		ch.ID, ch.UserID, ch.AuthKeyID, ch.Priv, ch.Pub, ch.Version, ch.Expires)
	return err
}

func (s *mysqlStore) TakeChallenge(ctx context.Context, id int64) (Challenge, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Challenge{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	var ch Challenge
	err = tx.QueryRowContext(ctx, `
SELECT srp_id, user_id, auth_key_id, b_priv, b_pub, version, expires
FROM user_password_srp WHERE srp_id = ? FOR UPDATE`, id).Scan(
		&ch.ID, &ch.UserID, &ch.AuthKeyID, &ch.Priv, &ch.Pub, &ch.Version, &ch.Expires)
	if err == sql.ErrNoRows {
		return Challenge{}, false, nil
	}
	if err != nil {
		return Challenge{}, false, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM user_password_srp WHERE srp_id = ?`, id); err != nil {
		return Challenge{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return Challenge{}, false, err
	}
	return ch, true, nil
}

func (s *mysqlStore) PutLogin(ctx context.Context, authKey, userID, at int64) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO user_password_login (auth_key_id, user_id, created_at) VALUES (?,?,?)
ON DUPLICATE KEY UPDATE user_id = VALUES(user_id), created_at = VALUES(created_at)`,
		authKey, userID, at)
	return err
}

func (s *mysqlStore) LoginUser(ctx context.Context, authKey int64) (int64, bool, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM user_password_login WHERE auth_key_id = ?`, authKey).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

func (s *mysqlStore) DeleteLogin(ctx context.Context, authKey int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM user_password_login WHERE auth_key_id = ?`, authKey)
	return err
}

func (s *mysqlStore) DeleteUserLogins(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM user_password_login WHERE user_id = ?`, userID)
	return err
}

func nz(b []byte) []byte {
	if b == nil {
		return []byte{}
	}
	return b
}
