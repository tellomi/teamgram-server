-- Per-topic notification settings (inputNotifyForumTopic).
-- Additive: a new table. user_notify_settings and its unique key are unchanged,
-- so a topic mute cannot overwrite the chat. Safe to run more than once.

CREATE TABLE IF NOT EXISTS `user_notify_topic_settings` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `user_id` bigint(20) NOT NULL,
  `peer_type` int(11) NOT NULL,
  `peer_id` bigint(20) NOT NULL,
  `top_msg_id` int(11) NOT NULL,
  `show_previews` int(11) NOT NULL DEFAULT '-1',
  `silent` int(11) NOT NULL DEFAULT '-1',
  `mute_until` int(11) NOT NULL DEFAULT '-1',
  `sound` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '-1',
  `stories_muted` int(11) NOT NULL DEFAULT '-1',
  `stories_hide_sender` int(11) NOT NULL DEFAULT '-1',
  `ios_sound` varchar(1024) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `android_sound` varchar(1024) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `other_sound` varchar(1024) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `stories_ios_sound` varchar(1024) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `stories_android_sound` varchar(1024) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `stories_other_sound` varchar(1024) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `deleted` tinyint(1) NOT NULL DEFAULT '0',
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `user_topic` (`user_id`,`peer_type`,`peer_id`,`top_msg_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
