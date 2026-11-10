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
-- spaces or tabs and an optional sign convert when they fit in signed bigint.
-- Leading spaces and tabs are removed with REGEXP_REPLACE. The pattern is a
-- character class, not TRIM(LEADING ' <tab>'), which deletes only that exact
-- two-character prefix and turns a 19-digit id with one leading space into NULL.
-- A bare CAST(JSON_UNQUOTE(JSON_EXTRACT(...)) AS SIGNED) is not used either:
-- it turns non-numeric strings into 0 and clips overflows.
--
-- Both columns and both indexes are one ALTER TABLE ... ALGORITHM=COPY,
-- LOCK=SHARED. Adding a STORED generated column rebuilds `messages` and
-- blocks writes for the copy (reads can continue). Doing it once copies the
-- table once. Run this file inside the cutover write-freeze window. The two
-- DROP INDEX statements above it only remove a legacy functional index of
-- the same name, and only while the generated column is still absent; those
-- drops are in-place and are not a second data copy.
--
-- An 8.0.46 database that already applied migrate-20261103-media-retention.sql
-- still has those functional indexes. A second run of this file sees the
-- columns and does not drop the new indexes or rebuild the table.
-- Safe to run more than once. The conditional statements use PREPARE because
-- each teamgram_add_column / teamgram_add_index call is its own ALTER, which
-- would rebuild `messages` twice.

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

SET @teamgram_stmt = (
  SELECT IF(
    (SELECT COUNT(*) FROM information_schema.COLUMNS
      WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'messages'
        AND COLUMN_NAME IN ('media_document_id', 'media_photo_id')) = 0,
    'ALTER TABLE `messages` ADD COLUMN `media_document_id` bigint GENERATED ALWAYS AS (CASE JSON_TYPE(JSON_EXTRACT(`message_data`, ''$.media.document.id''))
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
          CHAR_LENGTH(TRIM(LEADING ''0'' FROM TRIM(LEADING ''+'' FROM TRIM(LEADING ''-'' FROM REGEXP_REPLACE(JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.document.id'')), ''^[ \t]+'', ''''))))) < 19
          OR (
            CHAR_LENGTH(TRIM(LEADING ''0'' FROM TRIM(LEADING ''+'' FROM TRIM(LEADING ''-'' FROM REGEXP_REPLACE(JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.document.id'')), ''^[ \t]+'', ''''))))) = 19
            AND (
              (LEFT(REGEXP_REPLACE(JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.document.id'')), ''^[ \t]+'', ''''), 1) = ''-'' AND TRIM(LEADING ''0'' FROM TRIM(LEADING ''+'' FROM TRIM(LEADING ''-'' FROM REGEXP_REPLACE(JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.document.id'')), ''^[ \t]+'', '''')))) <= ''9223372036854775808'')
              OR (LEFT(REGEXP_REPLACE(JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.document.id'')), ''^[ \t]+'', ''''), 1) <> ''-'' AND TRIM(LEADING ''0'' FROM TRIM(LEADING ''+'' FROM TRIM(LEADING ''-'' FROM REGEXP_REPLACE(JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.document.id'')), ''^[ \t]+'', '''')))) <= ''9223372036854775807'')
            )
          )
        ),
        CAST(JSON_EXTRACT(`message_data`, ''$.media.document.id'') AS SIGNED),
        NULL)
      ELSE NULL
    END) STORED, ADD COLUMN `media_photo_id` bigint GENERATED ALWAYS AS (CASE JSON_TYPE(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id''))
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
          CHAR_LENGTH(TRIM(LEADING ''0'' FROM TRIM(LEADING ''+'' FROM TRIM(LEADING ''-'' FROM REGEXP_REPLACE(JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id'')), ''^[ \t]+'', ''''))))) < 19
          OR (
            CHAR_LENGTH(TRIM(LEADING ''0'' FROM TRIM(LEADING ''+'' FROM TRIM(LEADING ''-'' FROM REGEXP_REPLACE(JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id'')), ''^[ \t]+'', ''''))))) = 19
            AND (
              (LEFT(REGEXP_REPLACE(JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id'')), ''^[ \t]+'', ''''), 1) = ''-'' AND TRIM(LEADING ''0'' FROM TRIM(LEADING ''+'' FROM TRIM(LEADING ''-'' FROM REGEXP_REPLACE(JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id'')), ''^[ \t]+'', '''')))) <= ''9223372036854775808'')
              OR (LEFT(REGEXP_REPLACE(JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id'')), ''^[ \t]+'', ''''), 1) <> ''-'' AND TRIM(LEADING ''0'' FROM TRIM(LEADING ''+'' FROM TRIM(LEADING ''-'' FROM REGEXP_REPLACE(JSON_UNQUOTE(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id'')), ''^[ \t]+'', '''')))) <= ''9223372036854775807'')
            )
          )
        ),
        CAST(JSON_EXTRACT(`message_data`, ''$.media.photo_FLAGPHOTO.id'') AS SIGNED),
        NULL)
      ELSE NULL
    END) STORED, ADD INDEX `media_document_id` (`media_document_id`), ADD INDEX `media_photo_id` (`media_photo_id`), ALGORITHM=COPY, LOCK=SHARED',
    'SELECT 1'
  )
);
PREPARE teamgram_ps FROM @teamgram_stmt;
EXECUTE teamgram_ps;
DEALLOCATE PREPARE teamgram_ps;
