-- Private-chat inbox rows used to store messages.peer_id as the box owner
-- (the recipient) instead of the other user. getHistory, dialogs and most
-- deletes already key on dialog_id1/dialog_id2, so those paths keep
-- working. This updates the column in place so peer-scoped readers
-- (TTL backfill, report, wipe, media grouping) match the outbox and the
-- dialogs row. Nothing is deleted.
--
-- Applies to the unsharded `messages` table and to every `messages_N`
-- shard, if present. Backup tables (messages_bak_*) are not touched.
-- Saved Messages (sender_user_id = user_id) are left unchanged.
-- Safe to run more than once.

DROP PROCEDURE IF EXISTS tellomi_fix_inbox_private_peer;

DELIMITER $$

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

CALL tellomi_fix_inbox_private_peer();
DROP PROCEDURE IF EXISTS tellomi_fix_inbox_private_peer;
