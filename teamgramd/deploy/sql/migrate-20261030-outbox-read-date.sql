-- Read times live in message_read_outbox.read_outbox_max_date.
-- This file only creates the table when a database was built without
-- migrate-20241016.sql. It does not update or delete existing rows.
-- Apply it yourself after a backup. Do not run it against production from an agent.

CREATE TABLE IF NOT EXISTS `message_read_outbox` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `peer_dialog_id` bigint NOT NULL,
  `read_user_id` bigint NOT NULL,
  `read_outbox_max_id` int NOT NULL,
  `read_outbox_max_date` bigint NOT NULL,
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `user_id` (`user_id`,`peer_dialog_id`,`read_user_id`,`read_outbox_max_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
