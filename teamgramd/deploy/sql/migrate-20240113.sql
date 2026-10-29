CALL teamgram_add_column('messages', 'saved_peer_type', 'ALTER TABLE `messages` ADD `saved_peer_type` INT NOT NULL DEFAULT ''0'' AFTER `ttl_period`');
CALL teamgram_add_column('messages', 'saved_peer_id', 'ALTER TABLE `messages` ADD `saved_peer_id` BIGINT NOT NULL DEFAULT ''0'' AFTER `saved_peer_type`');
