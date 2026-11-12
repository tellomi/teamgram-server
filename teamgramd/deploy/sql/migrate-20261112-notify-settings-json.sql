-- peerNotifySettings gained NotificationSound and stories fields
-- (ios/android/other sound, stories_muted, stories_hide_sender, stories sounds).
-- The original columns stay the source for mute_until, show_previews, silent
-- and the legacy sound string used by push. settings_json holds the rest.
--
-- Rollback: CALL teamgram_drop_column('user_notify_settings', 'settings_json');
-- The column is nullable, so existing rows stay valid and old readers that
-- ignore it keep working. This file does not change TELLOMI_MESSAGE_EXPIRY.

CALL teamgram_add_column('user_notify_settings', 'settings_json', 'ALTER TABLE `user_notify_settings` ADD `settings_json` TEXT NULL');
