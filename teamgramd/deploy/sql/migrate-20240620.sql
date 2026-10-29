CALL teamgram_add_column('user_contacts', 'stories_hidden', 'ALTER TABLE `user_contacts` ADD `stories_hidden` BOOLEAN NOT NULL DEFAULT FALSE AFTER `close_friend`');
