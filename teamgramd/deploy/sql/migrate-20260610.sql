-- Participant admin rank. SelectList in chat_participants_dao.go reads this
-- column. A missing rank2 makes the participant list empty, and sending the
-- group-created service message then returns CHAT_WRITE_FORBIDDEN.
CALL teamgram_add_column('chat_participants', 'rank2', 'ALTER TABLE `chat_participants` ADD `rank2` VARCHAR(64) NOT NULL DEFAULT '''' AFTER `date2`');
