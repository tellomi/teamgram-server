CALL teamgram_add_column('users', 'premium_expire_date', 'ALTER TABLE `users` ADD `premium_expire_date` BIGINT NOT NULL DEFAULT ''0'' AFTER `premium`');
