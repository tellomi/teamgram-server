-- Service account 777000. Fresh installs already insert Tellomi via z_init.sql.
-- Databases that applied the older seed still have Telegram or Teamgram as
-- the contact name. Rename only those leftover labels. A row already named
-- Tellomi is left alone, so this file can run more than once.
-- The username table is unique. Skip that rename when @tellomi is taken.

SET @teamgram_stmt = 'UPDATE users SET first_name = ''Tellomi'' WHERE id = 777000 AND first_name IN (''Telegram'', ''Teamgram'')';
PREPARE teamgram_ps FROM @teamgram_stmt;
EXECUTE teamgram_ps;
DEALLOCATE PREPARE teamgram_ps;

SET @teamgram_stmt = 'UPDATE users SET username = ''tellomi'' WHERE id = 777000 AND username IN (''telegram'', ''teamgram'')';
PREPARE teamgram_ps FROM @teamgram_stmt;
EXECUTE teamgram_ps;
DEALLOCATE PREPARE teamgram_ps;

SET @teamgram_stmt = 'UPDATE username SET username = ''tellomi'' WHERE peer_id = 777000 AND username IN (''telegram'', ''teamgram'') AND NOT EXISTS (SELECT 1 FROM (SELECT id FROM username WHERE username = ''tellomi'' AND peer_id <> 777000) taken)';
PREPARE teamgram_ps FROM @teamgram_stmt;
EXECUTE teamgram_ps;
DEALLOCATE PREPARE teamgram_ps;
