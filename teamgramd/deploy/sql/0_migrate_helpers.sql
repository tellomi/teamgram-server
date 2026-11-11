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

-- teamgram_fail_if_set stops a migration with a readable message when detail
-- is not NULL. SIGNAL cannot run from a prepared statement (MySQL error
-- 1295), so migrations CALL this procedure instead. A NULL detail is a no-op.
DROP PROCEDURE IF EXISTS teamgram_fail_if_set$$
CREATE PROCEDURE teamgram_fail_if_set(IN detail VARCHAR(128), IN prefix VARCHAR(128))
BEGIN
  IF detail IS NOT NULL THEN
    SET @teamgram_fail = CONCAT(prefix, detail);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = @teamgram_fail;
  END IF;
END$$


-- Idempotent UPDATE of private-inbox messages.peer_id on every live
-- messages / messages_N shard. Backup tables (messages_bak_*) are not
-- touched. Saved Messages (sender_user_id = user_id) are left unchanged.
-- Called from migrate-20261111-inbox-private-peer.sql.
DROP PROCEDURE IF EXISTS tellomi_fix_inbox_private_peer$$
CREATE PROCEDURE tellomi_fix_inbox_private_peer()
BEGIN
  DECLARE done INT DEFAULT 0;
  DECLARE tname VARCHAR(64);
  DECLARE cur CURSOR FOR
    SELECT TABLE_NAME
    FROM information_schema.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND (TABLE_NAME = 'messages' OR TABLE_NAME REGEXP '^messages_[0-9]+$');
  DECLARE CONTINUE HANDLER FOR NOT FOUND SET done = 1;

  OPEN cur;
  read_loop: LOOP
    FETCH cur INTO tname;
    IF done THEN
      LEAVE read_loop;
    END IF;
    SET @tellomi_inbox_peer_sql = CONCAT(
      'UPDATE `', tname, '` ',
      'SET peer_id = sender_user_id ',
      'WHERE peer_type = 2 ',
      'AND peer_id = user_id ',
      'AND sender_user_id <> 0 ',
      'AND sender_user_id <> user_id'
    );
    PREPARE tellomi_inbox_peer_ps FROM @tellomi_inbox_peer_sql;
    EXECUTE tellomi_inbox_peer_ps;
    DEALLOCATE PREPARE tellomi_inbox_peer_ps;
  END LOOP;
  CLOSE cur;
END$$

DELIMITER ;
