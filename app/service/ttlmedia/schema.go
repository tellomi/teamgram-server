package ttlmedia

// schemaSQL matches teamgramd/deploy/sql/migrate-20261123-message-ttl-open.sql.
// A new table only: it does not rewrite messages, documents, or photos and
// does not take a lock on those tables. Safe to run more than once.
const schemaSQL = `
CREATE TABLE IF NOT EXISTS message_ttl_open (
  dialog_message_id BIGINT NOT NULL,
  ttl_seconds INT NOT NULL,
  opened_at INT NOT NULL,
  expire_at INT NOT NULL,
  media_kind VARCHAR(16) NOT NULL DEFAULT '',
  media_id BIGINT NOT NULL DEFAULT 0,
  stripped TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (dialog_message_id),
  KEY idx_ttl_due (stripped, expire_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
`
