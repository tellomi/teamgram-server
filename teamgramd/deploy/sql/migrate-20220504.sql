CALL teamgram_drop_index('documents', 'document_id');
CALL teamgram_add_index('documents', 'document_id', 'ALTER TABLE `documents` ADD UNIQUE `document_id` (`document_id`) USING BTREE');
CALL teamgram_add_index('user_contacts', 'owner_user_id', 'ALTER TABLE `user_contacts` ADD INDEX `owner_user_id` (`owner_user_id`)');
