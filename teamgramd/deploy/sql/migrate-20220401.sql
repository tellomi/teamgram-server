CALL teamgram_add_index('dialog_filters', 'user_id', 'ALTER TABLE `dialog_filters` ADD UNIQUE `user_id` (`user_id`, `dialog_filter_id`)');
