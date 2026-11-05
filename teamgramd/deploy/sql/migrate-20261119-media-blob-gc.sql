-- ADR-0083 B2. Queue of object keys whose last media_blob_refs row has
-- already been released. The blob row is deleted in the same transaction
-- that inserts here, so media_blobs never keeps ref_count = 0. The
-- retention task deletes the object afterwards. A failed delete leaves
-- this row and an orphan object, not a dangling reference.
-- Safe to run more than once.

CREATE TABLE IF NOT EXISTS `media_blob_gc` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `storage_class` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
  `file_path` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_media_blob_gc_path` (`storage_class`, `file_path`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
