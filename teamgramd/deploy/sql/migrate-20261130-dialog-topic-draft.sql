-- Forum topic drafts, one row per (user, peer, top_msg_id).
--
-- dialogs.draft_message_data stays the chat draft (top_msg_id = 0). This file
-- does not UPDATE or DELETE that column, so drafts already stored there are
-- kept and are read as the dialog draft. A topic draft cannot replace it.
-- Safe to run more than once.

CREATE TABLE IF NOT EXISTS `dialog_topic_drafts` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `user_id` bigint(20) NOT NULL,
  `peer_type` int(11) NOT NULL,
  `peer_id` bigint(20) NOT NULL,
  `top_msg_id` int(11) NOT NULL,
  `draft_type` int(11) NOT NULL DEFAULT '0',
  `draft_message_data` json NOT NULL,
  `date2` bigint(20) NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `user_peer_topic` (`user_id`,`peer_type`,`peer_id`,`top_msg_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
