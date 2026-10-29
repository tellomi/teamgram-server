-- Idempotent DDL helpers for MySQL 8.
-- MySQL 8 has no ADD COLUMN IF NOT EXISTS. These procedures no-op when the
-- column or index is already in the shape the migration wants, so a database
-- that applied some of these files by hand can run the whole set again.
-- docker-entrypoint-initdb.d runs this file before 1_teamgram.sql and the
-- migrate-*.sql files. teamgramd/deploy/migrate.sh applies it on every startup.

DROP PROCEDURE IF EXISTS teamgram_add_column;
DROP PROCEDURE IF EXISTS teamgram_drop_column;
DROP PROCEDURE IF EXISTS teamgram_add_index;
DROP PROCEDURE IF EXISTS teamgram_drop_index;
DROP PROCEDURE IF EXISTS teamgram_change_column;

DELIMITER $$

CREATE PROCEDURE teamgram_add_column(IN tbl VARCHAR(64), IN col VARCHAR(64), IN stmt TEXT)
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = tbl AND COLUMN_NAME = col
  ) THEN
    SET @teamgram_stmt = stmt;
    PREPARE teamgram_ps FROM @teamgram_stmt;
    EXECUTE teamgram_ps;
    DEALLOCATE PREPARE teamgram_ps;
  END IF;
END$$

CREATE PROCEDURE teamgram_drop_column(IN tbl VARCHAR(64), IN col VARCHAR(64))
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = tbl AND COLUMN_NAME = col
  ) THEN
    SET @teamgram_stmt = CONCAT('ALTER TABLE `', REPLACE(tbl, '`', '``'), '` DROP COLUMN `', REPLACE(col, '`', '``'), '`');
    PREPARE teamgram_ps FROM @teamgram_stmt;
    EXECUTE teamgram_ps;
    DEALLOCATE PREPARE teamgram_ps;
  END IF;
END$$

CREATE PROCEDURE teamgram_add_index(IN tbl VARCHAR(64), IN idx VARCHAR(64), IN stmt TEXT)
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = tbl AND INDEX_NAME = idx
  ) THEN
    SET @teamgram_stmt = stmt;
    PREPARE teamgram_ps FROM @teamgram_stmt;
    EXECUTE teamgram_ps;
    DEALLOCATE PREPARE teamgram_ps;
  END IF;
END$$

CREATE PROCEDURE teamgram_drop_index(IN tbl VARCHAR(64), IN idx VARCHAR(64))
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = tbl AND INDEX_NAME = idx
  ) THEN
    SET @teamgram_stmt = CONCAT('ALTER TABLE `', REPLACE(tbl, '`', '``'), '` DROP INDEX `', REPLACE(idx, '`', '``'), '`');
    PREPARE teamgram_ps FROM @teamgram_stmt;
    EXECUTE teamgram_ps;
    DEALLOCATE PREPARE teamgram_ps;
  END IF;
END$$

-- Runs stmt while old_col still exists. Used for CHANGE/MODIFY so a rename
-- that already happened is left alone, and a type change can be repeated.
CREATE PROCEDURE teamgram_change_column(IN tbl VARCHAR(64), IN old_col VARCHAR(64), IN stmt TEXT)
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = tbl AND COLUMN_NAME = old_col
  ) THEN
    SET @teamgram_stmt = stmt;
    PREPARE teamgram_ps FROM @teamgram_stmt;
    EXECUTE teamgram_ps;
    DEALLOCATE PREPARE teamgram_ps;
  END IF;
END$$

DELIMITER ;
