-- Audit log for @tellomi replies. Safe to run more than once.
-- The API key is not stored. See docs/api/tellomi-ai.md.

CREATE TABLE IF NOT EXISTS tellomi_ai_reply (
  id BIGINT NOT NULL AUTO_INCREMENT,
  user_id BIGINT NOT NULL,
  event_id VARCHAR(128) NOT NULL,
  model VARCHAR(64) NOT NULL,
  refused TINYINT NOT NULL,
  limited VARCHAR(16) NOT NULL,
  prompt TEXT NOT NULL,
  reply TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY tellomi_ai_reply_event (event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
