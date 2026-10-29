CALL teamgram_change_column('encrypted_files', 'file_size', 'ALTER TABLE `encrypted_files` CHANGE `file_size` `file_size` BIGINT NOT NULL');
CALL teamgram_change_column('documents', 'file_size', 'ALTER TABLE `documents` CHANGE `file_size` `file_size` BIGINT NOT NULL');
