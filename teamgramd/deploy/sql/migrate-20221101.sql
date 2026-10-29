CALL teamgram_add_column('auth_users', 'state', 'ALTER TABLE `auth_users` ADD `state` INT NOT NULL DEFAULT ''0'' AFTER `region`');
