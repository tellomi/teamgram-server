-- Stories: 24h posts, pins, albums, views, reactions, and read state.
-- owner_kind 1 is a user, 2 is a channel. Story ids are per owner.
-- Safe to run more than once.

CREATE TABLE IF NOT EXISTS story_counters (
  owner_kind TINYINT NOT NULL,
  owner_id BIGINT NOT NULL,
  next_story INT NOT NULL DEFAULT 0,
  next_album INT NOT NULL DEFAULT 0,
  PRIMARY KEY (owner_kind, owner_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS stories (
  owner_kind TINYINT NOT NULL,
  owner_id BIGINT NOT NULL,
  story_id INT NOT NULL,
  sender_user_id BIGINT NOT NULL,
  random_id BIGINT NOT NULL DEFAULT 0,
  date2 INT NOT NULL,
  expire_date INT NOT NULL,
  pinned TINYINT NOT NULL DEFAULT 0,
  deleted TINYINT NOT NULL DEFAULT 0,
  forwards INT NOT NULL DEFAULT 0,
  body MEDIUMBLOB NOT NULL,
  PRIMARY KEY (owner_kind, owner_id, story_id),
  KEY stories_random (owner_kind, owner_id, sender_user_id, random_id),
  KEY stories_expire (expire_date, deleted)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS story_views (
  owner_kind TINYINT NOT NULL,
  owner_id BIGINT NOT NULL,
  story_id INT NOT NULL,
  viewer_user_id BIGINT NOT NULL,
  date2 INT NOT NULL,
  reaction VARCHAR(64) NOT NULL DEFAULT '',
  stealth TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (owner_kind, owner_id, story_id, viewer_user_id),
  KEY story_views_viewer (viewer_user_id, date2)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS story_reads (
  user_id BIGINT NOT NULL,
  owner_kind TINYINT NOT NULL,
  owner_id BIGINT NOT NULL,
  max_read_id INT NOT NULL DEFAULT 0,
  hidden TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, owner_kind, owner_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS story_settings (
  user_id BIGINT NOT NULL,
  all_hidden TINYINT NOT NULL DEFAULT 0,
  stealth_active INT NOT NULL DEFAULT 0,
  stealth_cooldown INT NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS story_albums (
  owner_kind TINYINT NOT NULL,
  owner_id BIGINT NOT NULL,
  album_id INT NOT NULL,
  title VARCHAR(128) NOT NULL,
  order_index INT NOT NULL DEFAULT 0,
  story_ids TEXT NOT NULL,
  PRIMARY KEY (owner_kind, owner_id, album_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS story_pinned_top (
  owner_kind TINYINT NOT NULL,
  owner_id BIGINT NOT NULL,
  pos INT NOT NULL,
  story_id INT NOT NULL,
  PRIMARY KEY (owner_kind, owner_id, pos)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS story_reports (
  id BIGINT NOT NULL AUTO_INCREMENT,
  reporter_user_id BIGINT NOT NULL,
  owner_kind TINYINT NOT NULL,
  owner_id BIGINT NOT NULL,
  story_ids VARCHAR(255) NOT NULL DEFAULT '',
  option_text VARCHAR(64) NOT NULL DEFAULT '',
  message TEXT,
  date2 INT NOT NULL,
  PRIMARY KEY (id),
  KEY story_reports_owner (owner_kind, owner_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
