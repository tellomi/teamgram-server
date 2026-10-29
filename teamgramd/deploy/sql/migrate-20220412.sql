CALL teamgram_add_column('chats', 'noforwards', 'ALTER TABLE `chats` ADD `noforwards` BOOLEAN NOT NULL DEFAULT FALSE AFTER `deactivated`');
