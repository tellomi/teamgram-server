-- auth_users.activity_trusted marks date_active that a ping or RPC stored.
-- Rows from before this column existed stay 0. Their date_active is the
-- bind time, not inactivity, and account.setAuthorizationTTL must not
-- delete them on deploy. The first real request sets the flag and date_active
-- to now. Expiry starts only after that, once the session has been idle for
-- the full TTL.
--
-- Apply
--   Containers run teamgramd/deploy/migrate.sh before the processes start.
--   The script sources 0_migrate_helpers.sql, then each migrate-*.sql once.
--   By hand, from a machine that can reach MySQL:
--     mysql ... teamgram < teamgramd/deploy/sql/0_migrate_helpers.sql
--     mysql ... teamgram < teamgramd/deploy/sql/migrate-20261112-authorization-activity.sql
-- Safe to run more than once. This file does not delete rows and does not
-- log anyone out.

CALL teamgram_add_column('auth_users', 'activity_trusted', 'ALTER TABLE `auth_users` ADD `activity_trusted` TINYINT(1) NOT NULL DEFAULT 0 AFTER `date_active`');
