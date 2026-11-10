-- Message content wipe and media purge.
--
-- message_media_refs is one row per message box that still points at a
-- photo_id or document_id. Forwards that share an id are separate rows.
-- Re-uploads keep their own id and share the bytes through media_blobs.
-- media_purge_queue is the grace-period list. The worker re-checks holders
-- and only then releases media_blob_refs and deletes objects.
-- media_external_refs is a hold content-engine (or any other service) can
-- write when it still uses a teamgram photo_id or document_id. An empty
-- table means this database has no such hold.
--
-- These are new tables. Creating them does not rewrite messages,
-- channel_messages, documents, or photos, and does not take a lock on
-- those tables. Safe to run more than once.

CREATE TABLE IF NOT EXISTS `message_media_refs` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `box_kind` tinyint NOT NULL,
  `owner_id` bigint NOT NULL,
  `peer_type` int NOT NULL DEFAULT 0,
  `peer_id` bigint NOT NULL DEFAULT 0,
  `box_id` int NOT NULL,
  `media_kind` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `media_id` bigint NOT NULL,
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_box_media` (`box_kind`, `owner_id`, `peer_type`, `peer_id`, `box_id`, `media_kind`, `media_id`),
  KEY `idx_media` (`media_kind`, `media_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `media_purge_queue` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `media_kind` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `media_id` bigint NOT NULL,
  `enqueued_at` bigint NOT NULL,
  `available_at` bigint NOT NULL,
  `attempts` int NOT NULL DEFAULT 0,
  `last_error` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_purge_media` (`media_kind`, `media_id`),
  KEY `idx_purge_due` (`available_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `media_external_refs` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `source` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `media_kind` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `media_id` bigint NOT NULL,
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ext` (`source`, `media_kind`, `media_id`),
  KEY `idx_ext_media` (`media_kind`, `media_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
