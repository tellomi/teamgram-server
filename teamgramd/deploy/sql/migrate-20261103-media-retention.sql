-- Media retention (docs/media-retention.md).
-- expired_at is the unix time the retention sweeper removed the full-size
-- object; 0 means the media is still stored. upload.getFile answers
-- FILE_ID_INVALID for expired media.
-- The functional indexes let the sweeper find which message boxes still
-- reference a document or photo. Queries must repeat the expressions
-- verbatim (see app/service/media/retention/store.go).
-- Safe to run more than once.

CALL teamgram_add_column('documents', 'expired_at', 'ALTER TABLE `documents` ADD `expired_at` bigint NOT NULL DEFAULT ''0''');
CALL teamgram_add_column('photos', 'expired_at', 'ALTER TABLE `photos` ADD `expired_at` bigint NOT NULL DEFAULT ''0''');
CALL teamgram_add_column('encrypted_files', 'expired_at', 'ALTER TABLE `encrypted_files` ADD `expired_at` bigint NOT NULL DEFAULT ''0''');

CALL teamgram_add_index('messages', 'media_document_id', 'ALTER TABLE `messages` ADD INDEX `media_document_id` ((JSON_VALUE(`message_data`, ''$.media.document.id'' RETURNING SIGNED NULL ON EMPTY NULL ON ERROR)))');
CALL teamgram_add_index('messages', 'media_photo_id', 'ALTER TABLE `messages` ADD INDEX `media_photo_id` ((JSON_VALUE(`message_data`, ''$.media.photo_FLAGPHOTO.id'' RETURNING SIGNED NULL ON EMPTY NULL ON ERROR)))');
