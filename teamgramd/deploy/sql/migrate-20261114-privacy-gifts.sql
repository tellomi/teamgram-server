-- Gift privacy, disallowed gift types, and the sponsored-message switch.
-- Safe to run more than once.

CALL teamgram_add_column('user_global_privacy_settings', 'display_gifts_button', 'ALTER TABLE `user_global_privacy_settings` ADD `display_gifts_button` TINYINT(1) NOT NULL DEFAULT 0');
CALL teamgram_add_column('user_global_privacy_settings', 'disallowed_gifts', 'ALTER TABLE `user_global_privacy_settings` ADD `disallowed_gifts` VARCHAR(512) NOT NULL DEFAULT ''''');
CALL teamgram_add_column('user_global_privacy_settings', 'sponsored_enabled', 'ALTER TABLE `user_global_privacy_settings` ADD `sponsored_enabled` TINYINT(1) NOT NULL DEFAULT 0');
CALL teamgram_add_column('user_global_privacy_settings', 'noncontact_peers_paid_stars', 'ALTER TABLE `user_global_privacy_settings` ADD `noncontact_peers_paid_stars` BIGINT NOT NULL DEFAULT 0');
