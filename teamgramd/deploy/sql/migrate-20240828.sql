CALL teamgram_add_column('dialogs', 'unread_reactions_count', 'ALTER TABLE `dialogs` ADD `unread_reactions_count` INT NOT NULL DEFAULT ''0'' AFTER `unread_mentions_count`');
