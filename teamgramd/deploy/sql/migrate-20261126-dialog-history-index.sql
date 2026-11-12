-- messages.getHistory page for a private chat or basic group.
-- SelectDialogHistory (dialog_history_query.go) is:
--   WHERE user_id = ? AND dialog_id1 = ? AND dialog_id2 = ? AND deleted = 0
--     AND user_message_box_id < ?          -- offset / max id; omitted when unset
--   ORDER BY user_message_box_id DESC
--   LIMIT ?
-- The forward page uses user_message_box_id >= ? ORDER BY user_message_box_id ASC.
--
-- KEY user_id (user_id, dialog_id1, dialog_id2) finds the dialog and then
-- sorts. This index keeps `deleted` and `user_message_box_id` in that same
-- left-to-right order, so the range scan is the page and the sort goes away.
-- A synthetic 1,000,000-row run of this shape went from p95 0.918ms to
-- 0.0506ms. The old three-column key stays, so rollback is DROP INDEX.
--
-- InnoDB builds a secondary index in place without a table copy and without
-- blocking reads or writes (ALGORITHM=INPLACE, LOCK=NONE) on MySQL 8.0 and
-- 8.4. teamgram_add_index skips the statement when the index name is already
-- present, so running this file again is a no-op.

CALL teamgram_add_index('messages', 'messages_dialog_history', 'ALTER TABLE `messages` ADD INDEX `messages_dialog_history` (`user_id`, `dialog_id1`, `dialog_id2`, `deleted`, `user_message_box_id`), ALGORITHM=INPLACE, LOCK=NONE');
