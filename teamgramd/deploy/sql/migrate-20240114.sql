CALL teamgram_add_column('dialog_filters', 'is_chatlist', 'ALTER TABLE `dialog_filters` ADD `is_chatlist` BOOLEAN NOT NULL DEFAULT FALSE AFTER `dialog_filter_id`');
CALL teamgram_add_column('dialog_filters', 'has_my_invites', 'ALTER TABLE `dialog_filters` ADD `has_my_invites` INT NOT NULL DEFAULT ''0'' AFTER `is_chatlist`');
CALL teamgram_add_column('dialog_filters', 'slug', 'ALTER TABLE `dialog_filters` ADD `slug` VARCHAR(128) NOT NULL DEFAULT '''' AFTER `is_chatlist`');
CALL teamgram_add_column('dialog_filters', 'joined_by_slug', 'ALTER TABLE `dialog_filters` ADD `joined_by_slug` BOOLEAN NOT NULL DEFAULT FALSE AFTER `is_chatlist`');
