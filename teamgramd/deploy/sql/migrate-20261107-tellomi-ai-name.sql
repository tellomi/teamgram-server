-- Tellomi AI is its own account. User 777001 is @tellomi.
--
-- migrate-20261105-official-accounts.sql inserted 777001 with first_name
-- 'Tellomi', the same display name as 777002 (@tellomi_official) and the
-- 777000 service account. INSERT IGNORE does not change a row that already
-- exists, so this file updates existing rows.
--
-- 777000 stays the login-code account: first_name Tellomi, primary username
-- @admin. @tellomi resolves only to 777001. A name held by any other peer
-- stops the script (D-26). A second run changes nothing.

SET @bad = (SELECT username FROM username WHERE username = 'tellomi' AND NOT (peer_type = 2 AND peer_id IN (777000, 777001)) LIMIT 1);
CALL teamgram_fail_if_set(@bad, 'D-61: username tellomi is owned by: ');

SET @teamgram_stmt = 'UPDATE users SET username = ''admin'' WHERE id = 777000 AND username IN (''tellomi'', ''telegram'', ''teamgram'', '''')';
PREPARE teamgram_ps FROM @teamgram_stmt;
EXECUTE teamgram_ps;
DEALLOCATE PREPARE teamgram_ps;

SET @teamgram_stmt = 'UPDATE users SET first_name = ''Tellomi'' WHERE id = 777000 AND first_name IN (''Telegram'', ''Teamgram'')';
PREPARE teamgram_ps FROM @teamgram_stmt;
EXECUTE teamgram_ps;
DEALLOCATE PREPARE teamgram_ps;

SET @bad_user = (SELECT CAST(id AS CHAR) FROM users WHERE username = 'tellomi' AND id <> 777001 LIMIT 1);
CALL teamgram_fail_if_set(@bad_user, 'D-61: users.username tellomi is owned by: ');

INSERT IGNORE INTO users (id, user_type, access_hash, secret_key_id, first_name, last_name, username, phone, country_code, verified, support, about, is_bot) VALUES (777001, 2, 8100000000000777001, 0, 'Tellomi AI', '', 'tellomi', NULL, '', 1, 0, 'Tellomi 智能助手。直接发消息，即可获得解答、建议与灵感。', 0);

SET @teamgram_stmt = 'UPDATE users SET first_name = ''Tellomi AI'', username = ''tellomi'', about = ''Tellomi 智能助手。直接发消息，即可获得解答、建议与灵感。'' WHERE id = 777001';
PREPARE teamgram_ps FROM @teamgram_stmt;
EXECUTE teamgram_ps;
DEALLOCATE PREPARE teamgram_ps;

SET @teamgram_stmt = 'UPDATE username SET peer_type = 2, peer_id = 777001, editable = 1, active = 1, order2 = 2, deleted = 0 WHERE username = ''tellomi'' AND peer_id IN (777000, 777001)';
PREPARE teamgram_ps FROM @teamgram_stmt;
EXECUTE teamgram_ps;
DEALLOCATE PREPARE teamgram_ps;

INSERT IGNORE INTO username (username, peer_type, peer_id, editable, active, order2, deleted)
SELECT 'tellomi', 2, 777001, 1, 1, 2, 0
FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM username WHERE username = 'tellomi');
