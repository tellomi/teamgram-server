-- Indexed queue of messages still showing webPagePending after sendMessage.
-- The BFF sweeper also issues CREATE TABLE IF NOT EXISTS on startup.
-- Safe to run more than once. Does not alter the messages table.

CREATE TABLE IF NOT EXISTS `pending_webpage_jobs` (
  `user_id` bigint NOT NULL,
  `msg_id` int NOT NULL,
  `peer_type` int NOT NULL,
  `peer_id` bigint NOT NULL,
  `auth_key_id` bigint NOT NULL DEFAULT 0,
  `created_unix` bigint NOT NULL,
  PRIMARY KEY (`user_id`, `msg_id`),
  KEY `idx_pending_webpage_created` (`created_unix`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
