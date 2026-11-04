-- Idempotency for content-engine discover pushes. One row per engine event id.
-- Safe to run more than once. See docs/api/discover-ai.md.

CREATE TABLE IF NOT EXISTS discover_push_event (
  event_id VARCHAR(128) NOT NULL,
  user_id BIGINT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
