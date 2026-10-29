package channel

// schemaSQL is applied on MySQL open and kept in teamgramd/deploy/sql.
const schemaSQL = `
CREATE TABLE IF NOT EXISTS channels (
  id BIGINT NOT NULL,
  access_hash BIGINT NOT NULL,
  creator_user_id BIGINT NOT NULL,
  title VARCHAR(255) NOT NULL,
  about VARCHAR(255) NOT NULL DEFAULT '',
  username VARCHAR(32) NULL DEFAULT NULL,
  photo_id BIGINT NOT NULL DEFAULT 0,
  broadcast TINYINT NOT NULL DEFAULT 0,
  megagroup TINYINT NOT NULL DEFAULT 0,
  forum TINYINT NOT NULL DEFAULT 0,
  signatures TINYINT NOT NULL DEFAULT 0,
  pts INT NOT NULL DEFAULT 0,
  top_message INT NOT NULL DEFAULT 0,
  participants_count INT NOT NULL DEFAULT 0,
  date2 INT NOT NULL DEFAULT 0,
  migrated_from_chat_id BIGINT NOT NULL DEFAULT 0,
  migrated_from_max_id INT NOT NULL DEFAULT 0,
  ttl_period INT NOT NULL DEFAULT 0,
  address VARCHAR(255) NOT NULL DEFAULT '',
  default_banned_rights BIGINT NOT NULL DEFAULT 0,
  deleted TINYINT NOT NULL DEFAULT 0,
  version INT NOT NULL DEFAULT 1,
  PRIMARY KEY (id),
  UNIQUE KEY channels_username (username),
  KEY channels_migrated (migrated_from_chat_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS channel_participants (
  channel_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  role INT NOT NULL DEFAULT 0,
  state INT NOT NULL DEFAULT 0,
  admin_rights INT NOT NULL DEFAULT 0,
  banned_rights BIGINT NOT NULL DEFAULT 0,
  rank_name VARCHAR(16) NOT NULL DEFAULT '',
  inviter_user_id BIGINT NOT NULL DEFAULT 0,
  is_bot TINYINT NOT NULL DEFAULT 0,
  date2 INT NOT NULL DEFAULT 0,
  PRIMARY KEY (channel_id, user_id),
  KEY channel_participants_user (user_id, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS channel_messages (
  channel_id BIGINT NOT NULL,
  message_id INT NOT NULL,
  sender_user_id BIGINT NOT NULL,
  date2 INT NOT NULL,
  edit_date INT NOT NULL DEFAULT 0,
  random_id BIGINT NOT NULL DEFAULT 0,
  pts INT NOT NULL DEFAULT 0,
  deleted TINYINT NOT NULL DEFAULT 0,
  message_data MEDIUMBLOB NOT NULL,
  PRIMARY KEY (channel_id, message_id),
  KEY channel_messages_random (channel_id, sender_user_id, random_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS channel_pts_updates (
  channel_id BIGINT NOT NULL,
  pts INT NOT NULL,
  pts_count INT NOT NULL,
  kind VARCHAR(16) NOT NULL,
  message_id INT NOT NULL DEFAULT 0,
  message_ids VARCHAR(2048) NOT NULL DEFAULT '',
  payload MEDIUMBLOB NOT NULL,
  PRIMARY KEY (channel_id, pts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS channel_read_state (
  channel_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  read_inbox_max_id INT NOT NULL DEFAULT 0,
  read_outbox_max_id INT NOT NULL DEFAULT 0,
  unread_count INT NOT NULL DEFAULT 0,
  PRIMARY KEY (channel_id, user_id),
  KEY channel_read_state_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS channel_id_counter (
  id TINYINT NOT NULL,
  next_id BIGINT NOT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT IGNORE INTO channel_id_counter (id, next_id) VALUES (1, 1073741824);
`
