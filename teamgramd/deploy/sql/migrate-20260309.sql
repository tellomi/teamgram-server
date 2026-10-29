-- Indexes and auth_op_logs column widths.
-- chat_invite_participants.requested and approved_by are selected by the chat
-- DAO and indexed below, but no earlier migration added them. Without the
-- columns this file aborts, and later files (including rank2) never run.

CALL teamgram_add_column('chat_invite_participants', 'requested', 'ALTER TABLE `chat_invite_participants` ADD `requested` TINYINT(1) NOT NULL DEFAULT 0 AFTER `user_id`');
CALL teamgram_add_column('chat_invite_participants', 'approved_by', 'ALTER TABLE `chat_invite_participants` ADD `approved_by` BIGINT NOT NULL DEFAULT 0 AFTER `requested`');
CALL teamgram_add_index('auth_seq_updates', 'idx_auth_user_date', 'ALTER TABLE `auth_seq_updates` ADD KEY `idx_auth_user_date` (`auth_id`, `user_id`, `date2`, `seq`)');
CALL teamgram_add_index('auth_users', 'idx_user_deleted', 'ALTER TABLE `auth_users` ADD KEY `idx_user_deleted` (`user_id`, `deleted`)');

CREATE TABLE IF NOT EXISTS `auth_op_logs` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `auth_key_id` bigint(20) NOT NULL,
  `ip` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `op_type` int(11) NOT NULL DEFAULT '1',
  `log_text` varchar(512) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CALL teamgram_change_column('auth_op_logs', 'ip', 'ALTER TABLE `auth_op_logs` MODIFY COLUMN `ip` varchar(45) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '''' COMMENT ''client ip, IPv6 max 45'', MODIFY COLUMN `log_text` varchar(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '''' COMMENT ''log content'', COMMENT = ''Authorization operation log''');
CALL teamgram_add_index('chats', 'idx_creator_date', 'ALTER TABLE `chats` ADD KEY `idx_creator_date` (`creator_user_id`, `date`)');
CALL teamgram_add_index('chat_invite_participants', 'idx_chat_user', 'ALTER TABLE `chat_invite_participants` ADD KEY `idx_chat_user` (`chat_id`, `user_id`)');
CALL teamgram_add_index('chat_invite_participants', 'idx_chat_requested', 'ALTER TABLE `chat_invite_participants` ADD KEY `idx_chat_requested` (`chat_id`, `requested`)');
CALL teamgram_add_index('chat_participants', 'idx_user_state', 'ALTER TABLE `chat_participants` ADD KEY `idx_user_state` (`user_id`, `state`)');
