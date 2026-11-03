-- Account deletion queue. The wipe itself updates existing tables in short
-- batches. This table is the async object-storage trash list.
-- Safe to run more than once. Irreversible deletes of messages and profile
-- fields happen in the process, not in this file. Back up MySQL before
-- enabling a live object-storage pass (TELLOMI_ACCOUNT_GC_DRY_RUN=false).

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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
