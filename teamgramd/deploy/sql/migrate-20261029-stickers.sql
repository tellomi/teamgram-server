-- Openly licensed sticker and emoji packs.
-- Safe to run more than once. Does not touch the link-preview sticker_sets table
-- except that the import also upserts a public row there for /addstickers links.

CREATE TABLE IF NOT EXISTS tellomi_sticker_packs (
  id BIGINT NOT NULL,
  access_hash BIGINT NOT NULL,
  short_name VARCHAR(64) NOT NULL,
  title VARCHAR(128) NOT NULL,
  kind VARCHAR(64) NOT NULL,
  official TINYINT NOT NULL DEFAULT 1,
  masks TINYINT NOT NULL DEFAULT 0,
  emojis TINYINT NOT NULL DEFAULT 0,
  animated TINYINT NOT NULL DEFAULT 0,
  videos TINYINT NOT NULL DEFAULT 0,
  text_color TINYINT NOT NULL DEFAULT 0,
  featured TINYINT NOT NULL DEFAULT 0,
  installed_default TINYINT NOT NULL DEFAULT 0,
  license VARCHAR(64) NOT NULL,
  attribution VARCHAR(512) NOT NULL,
  source_url VARCHAR(255) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY tellomi_sticker_packs_short (short_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS tellomi_sticker_items (
  pack_id BIGINT NOT NULL,
  document_id BIGINT NOT NULL,
  access_hash BIGINT NOT NULL,
  emoticon VARCHAR(32) NOT NULL,
  sort_order INT NOT NULL,
  mime_type VARCHAR(64) NOT NULL,
  file_size INT NOT NULL,
  width INT NOT NULL,
  height INT NOT NULL,
  animated TINYINT NOT NULL DEFAULT 0,
  video TINYINT NOT NULL DEFAULT 0,
  custom_emoji TINYINT NOT NULL DEFAULT 0,
  file_name VARCHAR(128) NOT NULL,
  storage_class VARCHAR(16) NOT NULL,
  object_path VARCHAR(255) NOT NULL,
  asset_path VARCHAR(255) NOT NULL,
  keywords VARCHAR(512) NOT NULL DEFAULT '',
  PRIMARY KEY (pack_id, document_id),
  KEY tellomi_sticker_items_doc (document_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS tellomi_sticker_installs (
  user_id BIGINT NOT NULL,
  pack_id BIGINT NOT NULL,
  installed TINYINT NOT NULL,
  archived TINYINT NOT NULL DEFAULT 0,
  sort_order INT NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, pack_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS tellomi_sticker_recent (
  user_id BIGINT NOT NULL,
  document_id BIGINT NOT NULL,
  attached TINYINT NOT NULL DEFAULT 0,
  date_ts INT NOT NULL,
  PRIMARY KEY (user_id, document_id, attached)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS tellomi_sticker_faved (
  user_id BIGINT NOT NULL,
  document_id BIGINT NOT NULL,
  date_ts INT NOT NULL,
  PRIMARY KEY (user_id, document_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
