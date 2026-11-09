-- Sticker catalog generation and the GIF panel. Safe to run more than once.
-- The bff also creates these tables on startup.

CREATE TABLE IF NOT EXISTS `tellomi_catalog_version` (
  `name` VARCHAR(32) NOT NULL,
  `version` BIGINT NOT NULL,
  PRIMARY KEY (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `tellomi_gif_clips` (
  `id` BIGINT NOT NULL,
  `access_hash` BIGINT NOT NULL,
  `code` VARCHAR(64) NOT NULL,
  `title` VARCHAR(128) NOT NULL,
  `keywords` TEXT NOT NULL,
  `mime_type` VARCHAR(64) NOT NULL,
  `file_size` INT NOT NULL,
  `width` INT NOT NULL,
  `height` INT NOT NULL,
  `duration` DOUBLE NOT NULL DEFAULT 1,
  `file_name` VARCHAR(128) NOT NULL,
  `object_path` VARCHAR(255) NOT NULL,
  `thumb_path` VARCHAR(255) NOT NULL,
  `sort_order` INT NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `tellomi_gif_clips_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
