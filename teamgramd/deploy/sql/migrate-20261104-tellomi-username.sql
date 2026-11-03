-- D-27 / M5. Occupy @tellomi in the username table.
--
-- account.checkUsername reads this table (UserCheckAccountUsername →
-- UsernameDAO.SelectByUsername). z_init.sql inserts user 777000 with
-- users.username = 'tellomi' and does not insert a username row, so the
-- name was still free.
--
-- username.id is AUTO_INCREMENT (1_teamgram.sql). This file does not pick
-- an id. peer_type 2 is PEER_USER (proto/mtproto/peer_util.go). peer_id
-- 777000 is that service user (user_type 4, login codes).
--
-- The column collation is utf8mb4_unicode_ci, so Tellomi and TELLOMI are
-- the same key as tellomi. editable, active, and order2 are added by
-- migrate-20251219.sql, which sorts before this file.
--
-- migrate.sh runs every migrate-*.sql before z_init.sql. There is no
-- foreign key, so this row may be inserted before user 777000 exists.
-- A second run changes nothing. If another peer already holds the name,
-- the script stops and does not take it (D-26). The stop uses
-- teamgram_fail_if_set from 0_migrate_helpers.sql. SIGNAL cannot be
-- prepared (MySQL error 1295). migrate.sh installs that procedure first.
--
-- mysql-migrate and the teamgram entrypoint can apply this file at the
-- same time as migrate-20261105-official-accounts.sql, which moves
-- @tellomi from 777000 to the AI user 777001. Peer 777001 is that move,
-- not a squatter. This file must not point the row back at 777000.

SET @bad = (SELECT username FROM username WHERE username = 'tellomi' AND peer_id NOT IN (777000, 777001) LIMIT 1);
CALL teamgram_fail_if_set(@bad, 'D-27: username tellomi is owned by a peer other than user 777000: ');

SET @teamgram_stmt = 'UPDATE username SET username = ''tellomi'', peer_type = 2, editable = 1, active = 1, deleted = 0 WHERE username = ''tellomi'' AND peer_id = 777000';
PREPARE teamgram_ps FROM @teamgram_stmt;
EXECUTE teamgram_ps;
DEALLOCATE PREPARE teamgram_ps;

INSERT IGNORE INTO username (username, peer_type, peer_id, editable, active, order2, deleted)
SELECT 'tellomi', 2, 777000, 1, 1, 0, 0
FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM username WHERE username = 'tellomi');
