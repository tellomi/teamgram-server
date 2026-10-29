CALL teamgram_add_column('dialogs', 'ttl_period', 'ALTER TABLE `dialogs` ADD `ttl_period` INT NOT NULL DEFAULT ''0'' AFTER `has_scheduled`');
CALL teamgram_add_column('messages', 'ttl_period', 'ALTER TABLE `messages` ADD `ttl_period` INT NOT NULL DEFAULT ''0'' AFTER `date2`');
