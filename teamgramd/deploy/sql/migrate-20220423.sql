CALL teamgram_add_column('messages', 'reaction_unread', 'ALTER TABLE `messages` ADD `reaction_unread` BOOLEAN NOT NULL DEFAULT FALSE AFTER `reaction_date`');
CALL teamgram_add_column('messages', 'has_reaction', 'ALTER TABLE `messages` ADD `has_reaction` BOOLEAN NOT NULL DEFAULT FALSE AFTER `pinned`');
