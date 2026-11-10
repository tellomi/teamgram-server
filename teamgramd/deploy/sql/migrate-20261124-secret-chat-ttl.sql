-- Secret-chat self-destruct timer and the durable ciphertext expiry log.
--
-- migrate-20261120-secret-qts.sql creates secret_chats. This file is named
-- later so the column add runs after that table exists. Running it earlier
-- fails with ERROR 1146 (table secret_chats doesn't exist).
--
-- secret_message_expiry starts empty. It records only secret-chat ciphertext
-- the expiry switch has already observed. It does not UPDATE or DELETE
-- messages, channel_messages, documents, photos, or backups.
-- Safe to run more than once.

CALL teamgram_add_column('secret_chats', 'ttl_period', 'ALTER TABLE `secret_chats` ADD `ttl_period` int NOT NULL DEFAULT 0');

CREATE TABLE IF NOT EXISTS `secret_message_expiry` (
  `chat_id` int NOT NULL,
  `msg_id` int NOT NULL,
  `date2` int NOT NULL DEFAULT 0,
  `ttl_period` int NOT NULL DEFAULT 0,
  `file_id` bigint NOT NULL DEFAULT 0,
  `swept` tinyint NOT NULL DEFAULT 0,
  `user_a` bigint NOT NULL DEFAULT 0,
  `user_b` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`chat_id`, `msg_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
