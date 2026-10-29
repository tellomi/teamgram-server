CALL teamgram_add_column('chats', 'available_reactions', 'ALTER TABLE `chats` ADD `available_reactions` VARCHAR(128) NOT NULL DEFAULT '''' AFTER `migrated_to_access_hash`');
CALL teamgram_add_column('messages', 'reaction', 'ALTER TABLE `messages` ADD `reaction` VARCHAR(16) NOT NULL DEFAULT '''' AFTER `pinned`');
CALL teamgram_add_column('messages', 'reaction_date', 'ALTER TABLE `messages` ADD `reaction_date` BIGINT NOT NULL DEFAULT ''0'' AFTER `reaction`');
