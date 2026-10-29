CALL teamgram_add_column('users', 'personal_channel_id', 'ALTER TABLE `users` ADD `personal_channel_id` BIGINT NOT NULL DEFAULT ''0'' AFTER `birthday`');
