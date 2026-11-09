-- messages.sendMedia retries look up the sender's random_id. The column
-- already exists; this index is what keeps that lookup off a table scan.
-- InnoDB builds a secondary index in place without a table copy and without
-- blocking reads or writes (ALGORITHM=INPLACE, LOCK=NONE). teamgram_add_index
-- skips the statement when the index name is already present, so running
-- this file again is a no-op.

CALL teamgram_add_index('messages', 'sender_random_id', 'ALTER TABLE `messages` ADD INDEX `sender_random_id` (`sender_user_id`, `random_id`), ALGORITHM=INPLACE, LOCK=NONE');
