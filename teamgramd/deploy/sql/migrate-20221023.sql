CALL teamgram_add_column('users', 'emoji_status_document_id', 'ALTER TABLE `users` ADD `emoji_status_document_id` BIGINT NOT NULL DEFAULT ''0'' AFTER `archive_and_mute_new_noncontact_peers`');
CALL teamgram_add_column('users', 'emoji_status_until', 'ALTER TABLE `users` ADD `emoji_status_until` INT NOT NULL DEFAULT ''0'' AFTER `emoji_status_document_id`');
