-- channels.deleteMessages subtracts unread_count for members who have not
-- read the deleted id. The predicate is
--   WHERE channel_id = ? AND read_inbox_max_id < ? AND user_id <> ?
-- The primary key is (channel_id, user_id), so that range used to walk every
-- member. This index makes the range the rows whose inbox cursor is still
-- below the deleted id.
--
-- InnoDB builds it in place (ALGORITHM=INPLACE, LOCK=NONE) on MySQL 8.0 and
-- 8.4. teamgram_add_index skips the statement when the name exists, so
-- running this file again does not change rows.

CALL teamgram_add_index('channel_read_state', 'channel_read_inbox', 'ALTER TABLE `channel_read_state` ADD INDEX `channel_read_inbox` (`channel_id`, `read_inbox_max_id`), ALGORITHM=INPLACE, LOCK=NONE');
