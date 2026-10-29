CALL teamgram_add_column('chat_invite_participants', 'chat_id', 'ALTER TABLE `chat_invite_participants` ADD `chat_id` BIGINT NOT NULL DEFAULT 0 AFTER `id`');
