CALL teamgram_add_column('chats', 'ttl_period', 'ALTER TABLE `chats` ADD `ttl_period` INT NOT NULL DEFAULT ''0'' AFTER `noforwards`');
