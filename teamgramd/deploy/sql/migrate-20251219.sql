CALL teamgram_add_column('username', 'editable', 'ALTER TABLE `username` ADD `editable` BOOLEAN NOT NULL DEFAULT TRUE AFTER `peer_id`');
CALL teamgram_add_column('username', 'active', 'ALTER TABLE `username` ADD `active` BOOLEAN NOT NULL DEFAULT TRUE AFTER `editable`');
CALL teamgram_add_column('username', 'order2', 'ALTER TABLE `username` ADD `order2` BIGINT NOT NULL DEFAULT ''0'' AFTER `active`');
