-- Story deletions an offline client still has to learn about.
-- updates.getDifference reads this table. The live push of storyItemDeleted
-- is not durable. Safe to run more than once, and safe while stories are
-- being posted: this only creates a new table.

CREATE TABLE IF NOT EXISTS `story_deletions` (
  `viewer_user_id` bigint NOT NULL,
  `owner_kind` tinyint NOT NULL,
  `owner_id` bigint NOT NULL,
  `story_id` int NOT NULL,
  `deleted_at` int NOT NULL,
  PRIMARY KEY (`viewer_user_id`, `owner_kind`, `owner_id`, `story_id`),
  KEY `story_deletions_viewer_at` (`viewer_user_id`, `deleted_at`),
  KEY `story_deletions_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
