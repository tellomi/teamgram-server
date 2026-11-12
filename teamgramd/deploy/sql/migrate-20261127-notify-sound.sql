-- Notification sounds and story notify flags from inputPeerNotifySettings.
-- Safe to run more than once. Existing rows keep show_previews, silent,
-- mute_until and sound. The new columns default to unset (-1 or '').

CALL teamgram_add_column('user_notify_settings', 'stories_muted', 'ALTER TABLE `user_notify_settings` ADD `stories_muted` INT NOT NULL DEFAULT -1');
CALL teamgram_add_column('user_notify_settings', 'stories_hide_sender', 'ALTER TABLE `user_notify_settings` ADD `stories_hide_sender` INT NOT NULL DEFAULT -1');
CALL teamgram_add_column('user_notify_settings', 'ios_sound', 'ALTER TABLE `user_notify_settings` ADD `ios_sound` VARCHAR(1024) NOT NULL DEFAULT ''''');
CALL teamgram_add_column('user_notify_settings', 'android_sound', 'ALTER TABLE `user_notify_settings` ADD `android_sound` VARCHAR(1024) NOT NULL DEFAULT ''''');
CALL teamgram_add_column('user_notify_settings', 'other_sound', 'ALTER TABLE `user_notify_settings` ADD `other_sound` VARCHAR(1024) NOT NULL DEFAULT ''''');
CALL teamgram_add_column('user_notify_settings', 'stories_ios_sound', 'ALTER TABLE `user_notify_settings` ADD `stories_ios_sound` VARCHAR(1024) NOT NULL DEFAULT ''''');
CALL teamgram_add_column('user_notify_settings', 'stories_android_sound', 'ALTER TABLE `user_notify_settings` ADD `stories_android_sound` VARCHAR(1024) NOT NULL DEFAULT ''''');
CALL teamgram_add_column('user_notify_settings', 'stories_other_sound', 'ALTER TABLE `user_notify_settings` ADD `stories_other_sound` VARCHAR(1024) NOT NULL DEFAULT ''''');
