-- messages.sendMedia retries look up the sender's random_id. The column
-- already exists; this index is what keeps that lookup off a table scan.
-- Safe to run more than once.

CALL teamgram_add_index('messages', 'sender_random_id', 'ALTER TABLE `messages` ADD INDEX `sender_random_id` (`sender_user_id`, `random_id`)');
