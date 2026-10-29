CALL teamgram_add_column('users', 'color', 'ALTER TABLE `users` ADD `color` INT NOT NULL DEFAULT ''0'' AFTER `stories_max_id`');
CALL teamgram_add_column('users', 'color_background_emoji_id', 'ALTER TABLE `users` ADD `color_background_emoji_id` BIGINT NOT NULL DEFAULT ''0'' AFTER `color`');
CALL teamgram_add_column('users', 'profile_color', 'ALTER TABLE `users` ADD `profile_color` INT NOT NULL DEFAULT ''0'' AFTER `color_background_emoji_id`');
CALL teamgram_add_column('users', 'profile_color_background_emoji_id', 'ALTER TABLE `users` ADD `profile_color_background_emoji_id` BIGINT NOT NULL DEFAULT ''0'' AFTER `profile_color`');
