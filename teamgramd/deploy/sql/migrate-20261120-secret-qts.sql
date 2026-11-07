-- Secret chats and the per-user qts queue.
-- messages.requestEncryption used to keep the waiting chat in process
-- memory, so a restart or another phone instance could not accept it, and
-- updates.getDifference had no encrypted updates to return.
-- Safe to run more than once.

CREATE TABLE IF NOT EXISTS `secret_chats` (
  `chat_id` int NOT NULL,
  `access_hash` bigint NOT NULL,
  `date` int NOT NULL,
  `admin_id` bigint NOT NULL,
  `participant_id` bigint NOT NULL,
  `g_a` mediumblob NOT NULL,
  `g_b` mediumblob NULL,
  `key_fingerprint` bigint NOT NULL DEFAULT 0,
  `phase` tinyint NOT NULL,
  `history_deleted` tinyint NOT NULL DEFAULT 0,
  PRIMARY KEY (`chat_id`),
  KEY `idx_secret_chats_admin` (`admin_id`),
  KEY `idx_secret_chats_participant` (`participant_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `secret_qts_state` (
  `user_id` bigint NOT NULL,
  `qts` int NOT NULL,
  PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `secret_qts` (
  `user_id` bigint NOT NULL,
  `qts` int NOT NULL,
  `update_data` mediumtext NOT NULL,
  PRIMARY KEY (`user_id`, `qts`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `secret_sent` (
  `chat_id` int NOT NULL,
  `random_id` bigint NOT NULL,
  `date` int NOT NULL,
  `service` tinyint NOT NULL,
  `file_data` text NULL,
  PRIMARY KEY (`chat_id`, `random_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `secret_files` (
  `file_id` bigint NOT NULL,
  `chat_id` int NOT NULL,
  `file_data` text NOT NULL,
  PRIMARY KEY (`file_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
