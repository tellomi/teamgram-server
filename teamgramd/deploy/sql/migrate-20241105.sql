CALL teamgram_add_column('auth_users', 'android_push_session_id', 'ALTER TABLE `auth_users` ADD `android_push_session_id` BIGINT NOT NULL DEFAULT ''0'' AFTER `state`');
