-- Content-addressed media (ADR-0082 P1).
-- One physical object per sha256+size within a storage class. Each
-- document or photo row keeps its own id, access_hash and file_reference.
-- Safe to run more than once.

CREATE TABLE IF NOT EXISTS `media_blobs` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `sha256` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `file_size` bigint NOT NULL,
  `file_path` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `storage_class` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `ref_count` int NOT NULL DEFAULT 1,
  `origin_ref_id` bigint NOT NULL DEFAULT 0,
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_media_blobs_hash` (`storage_class`, `sha256`, `file_size`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `media_blob_refs` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `blob_id` bigint NOT NULL,
  `ref_kind` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
  `ref_id` bigint NOT NULL,
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_media_blob_refs_ref` (`ref_kind`, `ref_id`),
  KEY `idx_media_blob_refs_blob` (`blob_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Shared photo sizes (the original and the derived thumbs) keep the first
-- upload's object key. Download reads this before falling back to <type>/<id>.dat.
CREATE TABLE IF NOT EXISTS `media_photo_paths` (
  `photo_id` bigint NOT NULL,
  `size_type` varchar(8) COLLATE utf8mb4_unicode_ci NOT NULL,
  `file_path` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  PRIMARY KEY (`photo_id`, `size_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CALL teamgram_add_column('documents', 'sha256', 'ALTER TABLE `documents` ADD `sha256` char(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '''' AFTER `file_size`');
CALL teamgram_add_column('documents', 'blob_id', 'ALTER TABLE `documents` ADD `blob_id` bigint NOT NULL DEFAULT ''0'' AFTER `sha256`');
CALL teamgram_add_index('documents', 'idx_documents_sha256', 'ALTER TABLE `documents` ADD INDEX `idx_documents_sha256` (`sha256`, `file_size`, `mime_type`)');

CALL teamgram_add_column('photos', 'sha256', 'ALTER TABLE `photos` ADD `sha256` char(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT ''''');
CALL teamgram_add_column('photos', 'blob_id', 'ALTER TABLE `photos` ADD `blob_id` bigint NOT NULL DEFAULT ''0''');
