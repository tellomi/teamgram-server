-- Emoji reactions on private chats, basic groups, and channels.
-- dialog_message_id is the shared message id (channel message id for channels).
-- message_reaction_boxes maps each participant's own message id back to that key,
-- because private and basic-group box ids are not the same number for every user.
-- Safe to run more than once. The bff also creates these tables on startup.

CREATE TABLE IF NOT EXISTS message_reactions (
  id BIGINT NOT NULL AUTO_INCREMENT,
  dialog_id1 BIGINT NOT NULL,
  dialog_id2 BIGINT NOT NULL,
  dialog_message_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  emoticon VARCHAR(32) NOT NULL,
  big TINYINT NOT NULL DEFAULT 0,
  date2 INT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uniq_message_reaction (dialog_id1, dialog_id2, dialog_message_id, user_id, emoticon),
  KEY idx_message_reaction_msg (dialog_id1, dialog_id2, dialog_message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS message_reaction_boxes (
  user_id BIGINT NOT NULL,
  peer_type INT NOT NULL,
  peer_id BIGINT NOT NULL,
  msg_id INT NOT NULL,
  dialog_id1 BIGINT NOT NULL,
  dialog_id2 BIGINT NOT NULL,
  dialog_message_id BIGINT NOT NULL,
  PRIMARY KEY (user_id, peer_type, peer_id, msg_id),
  KEY idx_message_reaction_box_dialog (dialog_id1, dialog_id2, dialog_message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
