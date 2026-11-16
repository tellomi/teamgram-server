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
-- Apply this file before starting a process that inserts inbox_apply.
-- A missing table makes that insert fail. It does not report success.
-- The file only creates a table. It does not change existing rows.
-- Safe to run more than once.
--
-- Containers run teamgramd/deploy/migrate.sh before the processes start.

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
