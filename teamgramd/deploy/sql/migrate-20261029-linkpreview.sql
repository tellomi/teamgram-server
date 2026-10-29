-- Public sticker and custom-emoji packs for the link preview API.
-- The linkpreview process also creates this table on startup.
-- Safe to run more than once.

CREATE TABLE IF NOT EXISTS sticker_sets (
  id BIGINT NOT NULL AUTO_INCREMENT,
  short_name VARCHAR(64) NOT NULL,
  title VARCHAR(128) NOT NULL DEFAULT '',
  count INT NOT NULL DEFAULT 0,
  emoji TINYINT NOT NULL DEFAULT 0,
  thumb_path VARCHAR(255) NOT NULL DEFAULT '',
  public TINYINT NOT NULL DEFAULT 1,
  deleted TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY sticker_sets_short_name (short_name, emoji)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
