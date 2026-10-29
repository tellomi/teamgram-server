-- photo_sizes / video_sizes: cached sizes, drop legacy volume locations.
CALL teamgram_add_column('photo_sizes', 'cached_type', 'ALTER TABLE `photo_sizes` ADD `cached_type` INT NOT NULL DEFAULT ''0'' AFTER `file_path`');
CALL teamgram_add_column('photo_sizes', 'cached_bytes', 'ALTER TABLE `photo_sizes` ADD `cached_bytes` VARCHAR(4096) NOT NULL DEFAULT '''' AFTER `cached_type`');
CALL teamgram_drop_index('photo_sizes', 'volume_id');
CALL teamgram_drop_column('photo_sizes', 'volume_id');
CALL teamgram_drop_column('photo_sizes', 'local_id');
CALL teamgram_drop_column('photo_sizes', 'secret');
CALL teamgram_drop_column('photo_sizes', 'has_stripped');
CALL teamgram_drop_column('photo_sizes', 'stripped_bytes');
CALL teamgram_drop_index('video_sizes', 'volume_id');
CALL teamgram_drop_column('video_sizes', 'volume_id');
CALL teamgram_drop_column('video_sizes', 'local_id');
CALL teamgram_drop_column('video_sizes', 'secret');
