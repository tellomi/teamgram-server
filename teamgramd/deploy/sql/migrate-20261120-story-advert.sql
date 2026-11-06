-- Story advertisement index and cache-dirty rows.
--
-- Backup MySQL before applying this file on a database that already has
-- users. Take a mysqldump --single-transaction first and confirm it
-- restores. Do not run it from this pull request against production.
-- migrate.sh applies it on the next process start after that backup.
-- The statements are idempotent. Building users_stories_max reads the
-- users table once; on MariaDB 10.11 a secondary index is INPLACE.
-- It does not rewrite story rows and it does not touch Redis.
--
-- MariaDB 10.11 and MySQL 8.0 both show EXPLAIN type=ALL,
-- possible_keys=NULL for `stories_max_id <> 0` when that column has no
-- index. LIMIT does not stop that scan. `stories_max_id > 0` on
-- users_stories_max is type=range on both. MySQL 8 also has
-- EXPLAIN ANALYZE / FORMAT=TREE; MariaDB 10.11's ANALYZE output is not
-- that tree. Do not treat a MariaDB type=ALL plan as acceptable on MySQL 8.

CALL teamgram_add_index('users', 'users_stories_max', 'ALTER TABLE `users` ADD KEY `users_stories_max` (`stories_max_id`, `id`)');
CALL teamgram_add_index('stories', 'stories_owner_expire', 'ALTER TABLE `stories` ADD KEY `stories_owner_expire` (`owner_kind`, `deleted`, `expire_date`, `owner_id`, `story_id`)');

CREATE TABLE IF NOT EXISTS story_advert_dirty (
  user_id BIGINT NOT NULL,
  stories_max_id INT NOT NULL,
  updated_at INT NOT NULL,
  PRIMARY KEY (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
