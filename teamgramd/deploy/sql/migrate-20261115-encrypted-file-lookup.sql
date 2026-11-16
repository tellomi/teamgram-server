-- upload.getFile(inputEncryptedFileLocation) looks up encrypted_files by
-- encrypted_file_id. A retry of the same id must hit this key instead of
-- inserting a second row. teamgram_add_index skips the statement when the
-- name already exists. Safe to run more than once.

CALL teamgram_add_index('encrypted_files', 'encrypted_file_id', 'ALTER TABLE `encrypted_files` ADD UNIQUE KEY `encrypted_file_id` (`encrypted_file_id`)');
