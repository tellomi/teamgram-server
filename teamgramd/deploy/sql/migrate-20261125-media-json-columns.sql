-- Replace the JSON_VALUE functional indexes (MySQL 8.0.21) with generated
-- columns and ordinary secondary indexes. PolarDB's 8.0.13 kernel has
-- JSON_EXTRACT / JSON_UNQUOTE and STORED generated columns, and it does not
-- have JSON_VALUE, so the old indexes fail with ERROR 1064.
--
-- media_document_id / media_photo_id are bigint, the JSON_VALUE RETURNING
-- SIGNED type. The expression matches
-- JSON_VALUE(message_data, path RETURNING SIGNED NULL ON EMPTY NULL ON ERROR):
-- missing path, JSON null, arrays, objects, and failed conversions are NULL;
-- integers, in-range unsigned integers, booleans (true=1, false=0), and
-- in-range doubles (half-even) convert; digit strings with optional leading
-- space or tab and an optional sign convert when they fit in signed bigint.
-- A bare JSON_UNQUOTE(JSON_EXTRACT(...)) does not: CAST of that string turns
-- "abc" into 0 and clips overflows. Queries compare the column, which is what
-- the secondary index can serve.
--
-- An 8.0.46 database that already applied migrate-20261103-media-retention.sql
-- still has functional indexes of these names. Drop an index only while the
-- generated column is absent. After the column exists, the same name is the
-- secondary index and a later run must leave it in place.
-- Safe to run more than once. teamgram_add_column / teamgram_add_index no-op
-- when the column or index is already there. The DROP is a prepared statement
-- so it runs only in the legacy shape (see TestMigrateFilesAreIdempotent).

SET @teamgram_stmt = (
  SELECT IF(
    (SELECT COUNT(*) FROM information_schema.COLUMNS
      WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'messages' AND COLUMN_NAME = 'media_document_id') = 0
    AND (SELECT COUNT(*) FROM information_schema.STATISTICS
      WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'messages' AND INDEX_NAME = 'media_document_id') > 0,
    'ALTER TABLE `messages` DROP INDEX `media_document_id`',
    'SELECT 1'
  )
);
PREPARE teamgram_ps FROM @teamgram_stmt;
EXECUTE teamgram_ps;
DEALLOCATE PREPARE teamgram_ps;
CALL teamgram_add_column('messages', 'media_document_id', 'ALTER TABLE `messages` ADD COLUMN `media_document_id` bigint GENERATED ALWAYS AS (CASE JSON_TYPE(JSON_EXTRACT(`message_data`, ''$.media.document.id''))
      WHEN ''INTEGER'' THEN CAST(JSON_EXTRACT(`message_data`, ''$.media.document.id'') AS SIGNED)
      WHEN ''BOOLEAN'' THEN CAST(JSON_EXTRACT(`message_data`, ''$.media.document.id'') AS SIGNED)
      WHEN ''UNSIGNED INTEGER'' THEN IF(
        CHAR_LENGTH(JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.document.id''))) < 19
        OR (CHAR_LENGTH(JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.document.id''))) = 19 AND JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.document.id'')) <= ''9223372036854775807''),
        CAST(JSON_EXTRACT(`message_data`, ''$.media.document.id'') AS SIGNED),
        NULL)
      WHEN ''DOUBLE'' THEN IF(
        JSON_EXTRACT(`message_data`, ''$.media.document.id'') >= -9223372036854775808 AND JSON_EXTRACT(`message_data`, ''$.media.document.id'') <= 9223372036854775807,
        CAST(JSON_EXTRACT(`message_data`, ''$.media.document.id'') AS SIGNED),
        NULL)
      WHEN ''DECIMAL'' THEN IF(
        JSON_EXTRACT(`message_data`, ''$.media.document.id'') >= -9223372036854775808 AND JSON_EXTRACT(`message_data`, ''$.media.document.id'') <= 9223372036854775807,
        CAST(JSON_EXTRACT(`message_data`, ''$.media.document.id'') AS SIGNED),
        NULL)
      WHEN ''STRING'' THEN IF(
        JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.document.id'')) REGEXP ''^[ \t]*[+-]?[0-9]+$''
        AND (
          CHAR_LENGTH(TRIM(LEADING ''0'' FROM TRIM(LEADING ''+'' FROM TRIM(LEADING ''-'' FROM TRIM(LEADING '' \t'' FROM JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.document.id''))))))) < 19
          OR (
            CHAR_LENGTH(TRIM(LEADING ''0'' FROM TRIM(LEADING ''+'' FROM TRIM(LEADING ''-'' FROM TRIM(LEADING '' \t'' FROM JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.document.id''))))))) = 19
            AND (
              (LEFT(TRIM(LEADING '' \t'' FROM JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.document.id''))), 1) = ''-'' AND TRIM(LEADING ''0'' FROM TRIM(LEADING ''+'' FROM TRIM(LEADING ''-'' FROM TRIM(LEADING '' \t'' FROM JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.document.id'')))))) <= ''9223372036854775808'')
              OR (LEFT(TRIM(LEADING '' \t'' FROM JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.document.id''))), 1) <> ''-'' AND TRIM(LEADING ''0'' FROM TRIM(LEADING ''+'' FROM TRIM(LEADING ''-'' FROM TRIM(LEADING '' \t'' FROM JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.document.id'')))))) <= ''9223372036854775807'')
            )
          )
        ),
        CAST(JSON_EXTRACT(`message_data`, ''$.media.document.id'') AS SIGNED),
        NULL)
      ELSE NULL
    END) STORED');
CALL teamgram_add_index('messages', 'media_document_id', 'ALTER TABLE `messages` ADD INDEX `media_document_id` (`media_document_id`)');

SET @teamgram_stmt = (
  SELECT IF(
    (SELECT COUNT(*) FROM information_schema.COLUMNS
      WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'messages' AND COLUMN_NAME = 'media_photo_id') = 0
    AND (SELECT COUNT(*) FROM information_schema.STATISTICS
      WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'messages' AND INDEX_NAME = 'media_photo_id') > 0,
    'ALTER TABLE `messages` DROP INDEX `media_photo_id`',
    'SELECT 1'
  )
);
PREPARE teamgram_ps FROM @teamgram_stmt;
EXECUTE teamgram_ps;
DEALLOCATE PREPARE teamgram_ps;
CALL teamgram_add_column('messages', 'media_photo_id', 'ALTER TABLE `messages` ADD COLUMN `media_photo_id` bigint GENERATED ALWAYS AS (CASE JSON_TYPE(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id''))
      WHEN ''INTEGER'' THEN CAST(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id'') AS SIGNED)
      WHEN ''BOOLEAN'' THEN CAST(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id'') AS SIGNED)
      WHEN ''UNSIGNED INTEGER'' THEN IF(
        CHAR_LENGTH(JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id''))) < 19
        OR (CHAR_LENGTH(JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id''))) = 19 AND JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id'')) <= ''9223372036854775807''),
        CAST(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id'') AS SIGNED),
        NULL)
      WHEN ''DOUBLE'' THEN IF(
        JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id'') >= -9223372036854775808 AND JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id'') <= 9223372036854775807,
        CAST(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id'') AS SIGNED),
        NULL)
      WHEN ''DECIMAL'' THEN IF(
        JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id'') >= -9223372036854775808 AND JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id'') <= 9223372036854775807,
        CAST(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id'') AS SIGNED),
        NULL)
      WHEN ''STRING'' THEN IF(
        JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id'')) REGEXP ''^[ \t]*[+-]?[0-9]+$''
        AND (
          CHAR_LENGTH(TRIM(LEADING ''0'' FROM TRIM(LEADING ''+'' FROM TRIM(LEADING ''-'' FROM TRIM(LEADING '' \t'' FROM JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id''))))))) < 19
          OR (
            CHAR_LENGTH(TRIM(LEADING ''0'' FROM TRIM(LEADING ''+'' FROM TRIM(LEADING ''-'' FROM TRIM(LEADING '' \t'' FROM JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id''))))))) = 19
            AND (
              (LEFT(TRIM(LEADING '' \t'' FROM JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id''))), 1) = ''-'' AND TRIM(LEADING ''0'' FROM TRIM(LEADING ''+'' FROM TRIM(LEADING ''-'' FROM TRIM(LEADING '' \t'' FROM JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id'')))))) <= ''9223372036854775808'')
              OR (LEFT(TRIM(LEADING '' \t'' FROM JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id''))), 1) <> ''-'' AND TRIM(LEADING ''0'' FROM TRIM(LEADING ''+'' FROM TRIM(LEADING ''-'' FROM TRIM(LEADING '' \t'' FROM JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id'')))))) <= ''9223372036854775807'')
            )
          )
        ),
        CAST(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id'') AS SIGNED),
        NULL)
      ELSE NULL
    END) STORED');
CALL teamgram_add_index('messages', 'media_photo_id', 'ALTER TABLE `messages` ADD INDEX `media_photo_id` (`media_photo_id`)');

