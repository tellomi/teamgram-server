-- Story advertisement tables. Not a migrate-*.sql file.
-- teamgramd/deploy/migrate.sh and the container entrypoint do not run it.
-- teamgramd/deploy/apply-gated-sql.sh runs it only after
-- TEAMGRAM_SQL_BACKUP_CONFIRMED=yes.
--
-- The users index build reads the users table once. On MariaDB 10.11 a
-- secondary index is INPLACE. Take a restorable mysqldump before this file
-- runs. This repository does not run it.
--
-- The backfill below reads currently unexpired story rows once, at the
-- moment the operator runs the script. Process startup does not run it.
-- Plans below were read on MariaDB 10.11 only. MySQL 8 was not measured.

CALL teamgram_add_index('users', 'users_stories_max', 'ALTER TABLE `users` ADD KEY `users_stories_max` (`stories_max_id`, `id`)');

CREATE TABLE IF NOT EXISTS story_advert_dirty (
  user_id BIGINT NOT NULL,
  stories_max_id INT NOT NULL,
  updated_at INT NOT NULL,
  PRIMARY KEY (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS story_live_owner (
  owner_id BIGINT NOT NULL,
  live_max INT NOT NULL,
  PRIMARY KEY (owner_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO story_live_owner (owner_id, live_max)
SELECT owner_id, MAX(story_id) FROM stories
WHERE owner_kind = 1 AND deleted = 0 AND expire_date > UNIX_TIMESTAMP()
GROUP BY owner_id
ON DUPLICATE KEY UPDATE live_max = VALUES(live_max);
