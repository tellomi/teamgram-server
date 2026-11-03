-- Session flags for account.changeAuthorizationSettings and
-- account.getAuthorizations (authorization constructor).
--
-- encrypted_requests_disabled  authorization flag, 0 = this session accepts secret chats
-- call_requests_disabled       authorization flag, 0 = this session accepts calls
-- unconfirmed                  authorization.unconfirmed. A newly inserted session is
--                              marked 1 when the user already has another live session.
--                              confirmed on account.changeAuthorizationSettings sets it back to 0.
--                              Rows that already exist stay confirmed and keep accepting
--                              secret chats and calls.
--
-- Apply
--   Containers run teamgramd/deploy/migrate.sh before the processes start.
--   The script sources 0_migrate_helpers.sql, then each migrate-*.sql once,
--   and records the filename in schema_migrations.
--   By hand, from a machine that can reach MySQL:
--     mysql ... teamgram < teamgramd/deploy/sql/0_migrate_helpers.sql
--     mysql ... teamgram < teamgramd/deploy/sql/migrate-20261103-authorization-settings.sql
-- Safe to run more than once. teamgram_add_column does nothing when the column exists.
-- Restart authsession after the columns exist. A binary that selects these
-- columns before this file has run still serves the device list (flags read
-- as off) and account.changeAuthorizationSettings returns INTERNAL_SERVER_ERROR
-- until the columns are there.

CALL teamgram_add_column('auth_users', 'encrypted_requests_disabled', 'ALTER TABLE `auth_users` ADD `encrypted_requests_disabled` TINYINT(1) NOT NULL DEFAULT 0 AFTER `android_push_session_id`');
CALL teamgram_add_column('auth_users', 'call_requests_disabled', 'ALTER TABLE `auth_users` ADD `call_requests_disabled` TINYINT(1) NOT NULL DEFAULT 0 AFTER `encrypted_requests_disabled`');
CALL teamgram_add_column('auth_users', 'unconfirmed', 'ALTER TABLE `auth_users` ADD `unconfirmed` TINYINT(1) NOT NULL DEFAULT 0 AFTER `call_requests_disabled`');
