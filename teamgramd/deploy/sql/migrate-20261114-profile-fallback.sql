-- Public profile photo shown when profile-photo privacy hides the main one.
-- Safe to run more than once.

CALL teamgram_add_column('user_profile_photos', 'fallback', 'ALTER TABLE `user_profile_photos` ADD `fallback` tinyint(1) NOT NULL DEFAULT 0');
