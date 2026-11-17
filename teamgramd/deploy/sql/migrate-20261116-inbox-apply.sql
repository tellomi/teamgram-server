-- Inbox delivery marker. The message row, the dialog unread increment, and
-- this row commit together. applied is 1 in that insert. A later replay
-- sees the row and does not add unread again.
--
-- An older process does not read this table. Rolling the container back
-- leaves the table in place. The older process still finds the message by
-- random_id and does not add unread a second time. A transaction that
-- rolls back leaves no message and no row, so the older process inserts
-- the message and adds unread once.
--
-- Apply this file before starting a process that inserts these tables.
-- A missing table makes that insert fail. It does not report success.
-- The file only creates tables and one trigger. It does not change existing rows.
-- Safe to run more than once.
--
-- Containers run teamgramd/deploy/migrate.sh before the processes start.
--
-- inbox_random records a random_id the new process has already stored.
-- The previous container's sendMessageToInbox does not look the id up. It
-- allocates a new box id, inserts, and then adds unread. The trigger turns
-- that second insert into a duplicate-key error before the unread update.
-- The current inbox consumer logs that error and continues, so the extra
-- row and the extra unread are not written. A random_id of 0 is not recorded
-- and is not rejected. Messages the previous container stored itself have no
-- inbox_random row, so its own insert is unchanged.

CREATE TABLE IF NOT EXISTS `inbox_random` (
  `user_id` bigint(20) NOT NULL,
  `random_id` bigint(20) NOT NULL,
  `sender_user_id` bigint(20) NOT NULL,
  `box_id` int(11) NOT NULL,
  PRIMARY KEY (`user_id`, `random_id`, `sender_user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- DROP TRIGGER cannot run as a prepared statement (MySQL error 1295).
-- The helper drops it as a normal statement. Creating it again does not
-- change message rows.
CALL teamgram_drop_inbox_random_guard();

DELIMITER $$
CREATE TRIGGER `messages_inbox_random_guard`
BEFORE INSERT ON `messages`
FOR EACH ROW
BEGIN
  IF NEW.random_id <> 0 AND EXISTS (
    SELECT 1 FROM `inbox_random`
    WHERE `user_id` = NEW.user_id
      AND `random_id` = NEW.random_id
      AND `sender_user_id` = NEW.sender_user_id
  ) THEN
    SIGNAL SQLSTATE '23000'
      SET MYSQL_ERRNO = 1062,
          MESSAGE_TEXT = 'Duplicate inbox random_id';
  END IF;
END$$
DELIMITER ;

CREATE TABLE IF NOT EXISTS `inbox_apply` (
  `user_id` bigint(20) NOT NULL,
  `box_id` int(11) NOT NULL,
  `peer_type` int(11) NOT NULL,
  `peer_id` bigint(20) NOT NULL,
  `unread_delta` int(11) NOT NULL,
  `mentions_delta` int(11) NOT NULL,
  `pts` int(11) NOT NULL,
  `pts_count` int(11) NOT NULL,
  `date2` bigint(20) NOT NULL,
  `applied` tinyint(1) NOT NULL DEFAULT '0',
  PRIMARY KEY (`user_id`, `box_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
