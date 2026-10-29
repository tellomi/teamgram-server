-- Per-user account.getReactionsNotifySettings / account.setReactionsNotifySettings.
-- settings_blob is a layer 232 reactionsNotifySettings. Safe to run more than once.
-- The bff process also creates this table on startup.

CREATE TABLE IF NOT EXISTS user_reactions_notify (
  user_id BIGINT NOT NULL,
  settings_blob MEDIUMBLOB NOT NULL,
  PRIMARY KEY (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
