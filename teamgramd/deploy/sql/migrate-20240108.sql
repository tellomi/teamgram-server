CALL teamgram_add_column('user_contacts', 'close_friend', 'ALTER TABLE `user_contacts` ADD `close_friend` BOOLEAN NOT NULL DEFAULT FALSE AFTER `mutual`');
