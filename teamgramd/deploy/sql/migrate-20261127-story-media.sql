-- Photo and document ids stored on stories, so upload.getFile can refuse
-- an expired story without scanning story bodies. Safe to run more than once.

CREATE TABLE IF NOT EXISTS `story_media` (
  `media_id` bigint NOT NULL,
  `owner_kind` tinyint NOT NULL,
  `owner_id` bigint NOT NULL,
  `story_id` int NOT NULL,
  PRIMARY KEY (`media_id`, `owner_kind`, `owner_id`, `story_id`),
  KEY `story_media_story` (`owner_kind`, `owner_id`, `story_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
