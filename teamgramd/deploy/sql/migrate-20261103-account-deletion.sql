-- Web and in-app account deletion.
-- password_set_at records when the cloud-password verifier last changed.
-- Zero means the row predates this column and is treated as older than 7 days.
-- account_deletion_pending is the 7-day wait from account.deleteAccount when
-- a cloud password is set and the proof is omitted.
-- Safe to run more than once.

CALL teamgram_add_column('user_passwords', 'password_set_at', 'ALTER TABLE `user_passwords` ADD `password_set_at` BIGINT NOT NULL DEFAULT 0');

CREATE TABLE IF NOT EXISTS account_deletion_pending (
  user_id BIGINT NOT NULL,
  delete_at BIGINT NOT NULL,
  reason VARCHAR(128) NOT NULL DEFAULT '',
  created_at BIGINT NOT NULL,
  PRIMARY KEY (user_id),
  KEY delete_at (delete_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
