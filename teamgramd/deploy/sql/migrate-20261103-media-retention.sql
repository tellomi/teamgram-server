-- Media retention (docs/media-retention.md).
-- expired_at is the unix time the retention sweeper removed the full-size
-- object; 0 means the media is still stored. upload.getFile answers
-- FILE_ID_INVALID for expired media.
-- Which message boxes still reference a document or photo is answered by
-- generated columns media_document_id and media_photo_id. Those are added
-- in migrate-20261125-media-json-columns.sql, including on a database that
-- already has the old functional indexes from an earlier copy of this file.
-- Safe to run more than once.

CALL teamgram_add_column('documents', 'expired_at', 'ALTER TABLE `documents` ADD `expired_at` bigint NOT NULL DEFAULT ''0''');
CALL teamgram_add_column('photos', 'expired_at', 'ALTER TABLE `photos` ADD `expired_at` bigint NOT NULL DEFAULT ''0''');
CALL teamgram_add_column('encrypted_files', 'expired_at', 'ALTER TABLE `encrypted_files` ADD `expired_at` bigint NOT NULL DEFAULT ''0''');
