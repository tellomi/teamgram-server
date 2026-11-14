-- Communities (channels linked as one community chat).
-- Safe to run more than once. Does not rewrite chat or message tables.

CREATE TABLE IF NOT EXISTS `communities` (
  `id` bigint NOT NULL,
  `access_hash` bigint NOT NULL,
  `owner_user_id` bigint NOT NULL,
  `title` varchar(128) NOT NULL,
  `about` varchar(255) NOT NULL DEFAULT '',
  `collapsed` tinyint NOT NULL DEFAULT 0,
  `created_at` int NOT NULL,
  PRIMARY KEY (`id`),
  KEY `communities_owner` (`owner_user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `community_links` (
  `community_id` bigint NOT NULL,
  `peer_type` int NOT NULL,
  `peer_id` bigint NOT NULL,
  `visible` tinyint NOT NULL DEFAULT 0,
  `pending` tinyint NOT NULL DEFAULT 0,
  `requested_by` bigint NOT NULL,
  `created_at` int NOT NULL,
  PRIMARY KEY (`community_id`, `peer_type`, `peer_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `community_members` (
  `community_id` bigint NOT NULL,
  `user_id` bigint NOT NULL,
  `banned` tinyint NOT NULL DEFAULT 0,
  PRIMARY KEY (`community_id`, `user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
