-- Indexed queue of messages still showing webPagePending after sendMessage.
-- The BFF sweeper also issues CREATE TABLE IF NOT EXISTS and idempotent
-- ALTER TABLE ADD COLUMN on startup (ignores MySQL 1060 duplicate column).
-- Safe to run more than once. Does not alter the messages table.
-- attempts / next_retry_unix bound sweeper retries: after 10 attempts or
-- 1 hour the job is dropped. A message that no longer exists or is no
-- longer pending is deleted immediately.

CREATE TABLE IF NOT EXISTS `pending_webpage_jobs` (
  `user_id` bigint NOT NULL,
  `msg_id` int NOT NULL,
  `peer_type` int NOT NULL,
  `peer_id` bigint NOT NULL,
  `auth_key_id` bigint NOT NULL DEFAULT 0,
  `created_unix` bigint NOT NULL,
  `attempts` int NOT NULL DEFAULT 0,
  `next_retry_unix` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`user_id`, `msg_id`),
  KEY `idx_pending_webpage_created` (`created_unix`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Idempotent for a table created before these columns existed.
-- MySQL 8 has no ADD COLUMN IF NOT EXISTS, so we only ALTER when missing.
SET @db := DATABASE();
SET @sql := (
  SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE `pending_webpage_jobs` ADD COLUMN `attempts` int NOT NULL DEFAULT 0',
    'SELECT 1'
  )
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = @db AND TABLE_NAME = 'pending_webpage_jobs' AND COLUMN_NAME = 'attempts'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := (
  SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE `pending_webpage_jobs` ADD COLUMN `next_retry_unix` bigint NOT NULL DEFAULT 0',
    'SELECT 1'
  )
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = @db AND TABLE_NAME = 'pending_webpage_jobs' AND COLUMN_NAME = 'next_retry_unix'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
