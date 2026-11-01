-- Cloud password (Telegram 2FA / SRP).
-- user_passwords holds the verifier, salts, recovery email, and reset date.
-- user_password_srp is one attempt: srp_id and B are deleted when used.
-- user_password_login remembers which user an auth key must still unlock
-- so auth.signIn can return SESSION_PASSWORD_NEEDED with no user id in the text.
-- Safe to run more than once.

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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
