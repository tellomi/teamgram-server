CALL teamgram_add_column('chats', 'available_reactions_type', 'ALTER TABLE `chats` ADD `available_reactions_type` INT NOT NULL DEFAULT ''0'' AFTER `migrated_to_access_hash`');
