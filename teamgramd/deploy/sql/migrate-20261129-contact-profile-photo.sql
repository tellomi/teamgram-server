-- Personal profile photo one user sets for a contact.
-- photos.uploadContactProfilePhoto save. Safe to run more than once.
-- Does not delete or rewrite existing rows.

CREATE TABLE IF NOT EXISTS `user_contact_photos` (
  `owner_user_id` bigint NOT NULL,
  `contact_user_id` bigint NOT NULL,
  `photo_id` bigint NOT NULL,
  `photo` mediumblob NOT NULL,
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`owner_user_id`, `contact_user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
