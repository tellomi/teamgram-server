-- user_global_privacy_settings and users.birthday
CALL teamgram_add_column('user_global_privacy_settings', 'keep_archived_unmuted', 'ALTER TABLE `user_global_privacy_settings` ADD `keep_archived_unmuted` BOOLEAN NOT NULL DEFAULT FALSE AFTER `archive_and_mute_new_noncontact_peers`');
CALL teamgram_add_column('user_global_privacy_settings', 'keep_archived_folders', 'ALTER TABLE `user_global_privacy_settings` ADD `keep_archived_folders` BOOLEAN NOT NULL DEFAULT FALSE AFTER `keep_archived_unmuted`');
CALL teamgram_add_column('user_global_privacy_settings', 'hide_read_marks', 'ALTER TABLE `user_global_privacy_settings` ADD `hide_read_marks` BOOLEAN NOT NULL DEFAULT FALSE AFTER `keep_archived_folders`');
CALL teamgram_add_column('user_global_privacy_settings', 'new_noncontact_peers_require_premium', 'ALTER TABLE `user_global_privacy_settings` ADD `new_noncontact_peers_require_premium` BOOLEAN NOT NULL DEFAULT FALSE AFTER `hide_read_marks`');
CALL teamgram_add_column('users', 'birthday', 'ALTER TABLE `users` ADD `birthday` CHAR(10) NOT NULL DEFAULT '''' AFTER `profile_color_background_emoji_id`');
