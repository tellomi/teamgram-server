-- Global post search stores the message text next to the protobuf blob
-- so channels.searchPosts can use LIKE, and records a channel ban or
-- restriction so those channels stay out of search.
-- Safe to run more than once.

CALL teamgram_add_column('channels', 'restricted', 'ALTER TABLE channels ADD COLUMN restricted TINYINT NOT NULL DEFAULT 0');
CALL teamgram_add_column('channels', 'banned', 'ALTER TABLE channels ADD COLUMN banned TINYINT NOT NULL DEFAULT 0');
CALL teamgram_add_column('channel_messages', 'message', 'ALTER TABLE channel_messages ADD COLUMN message TEXT NULL');
