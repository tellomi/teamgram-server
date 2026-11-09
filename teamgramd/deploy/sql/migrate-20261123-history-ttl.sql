-- Auto-delete timers. Safe to run more than once.
--
-- messages.ttl_period already exists. The index lets the expiry worker find
-- due rows without a full table scan. channel_messages.ttl_period is the
-- same stamp for channels and supergroups; the column is also added when the
-- channel service opens MySQL, so a database created before this file still
-- converges.
--
-- Adding the messages index on a large table is online (INPLACE, LOCK=NONE)
-- on MySQL 8. It does not rewrite message bodies and does not delete rows.

CALL teamgram_add_index('messages', 'messages_ttl_due', 'ALTER TABLE `messages` ADD INDEX `messages_ttl_due` (`ttl_period`, `date2`), ALGORITHM=INPLACE, LOCK=NONE');
CALL teamgram_add_column('channel_messages', 'ttl_period', 'ALTER TABLE `channel_messages` ADD `ttl_period` INT NOT NULL DEFAULT 0');
CALL teamgram_add_index('channel_messages', 'channel_messages_ttl', 'ALTER TABLE `channel_messages` ADD INDEX `channel_messages_ttl` (`ttl_period`, `date2`), ALGORITHM=INPLACE, LOCK=NONE');
