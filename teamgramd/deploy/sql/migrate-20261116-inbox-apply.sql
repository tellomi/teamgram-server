-- Inbox delivery marker. The message row and this row commit together.
-- applied stays 0 until the dialog unread increment commits in the same
-- transaction as the 0 -> 1 change. A crash between those two steps leaves
-- applied = 0, and the retry adds the unread once. Existing history has no
-- row here, so a replay of an older message does not add unread.
--
-- Apply
--   Containers run teamgramd/deploy/migrate.sh before the processes start.
--   The script sources 0_migrate_helpers.sql, then each migrate-*.sql once.
-- Safe to run more than once.

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

CREATE TABLE IF NOT EXISTS `inbox_pts_lock` (
  `user_id` bigint(20) NOT NULL,
  PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
