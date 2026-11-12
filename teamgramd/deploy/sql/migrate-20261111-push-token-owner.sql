-- Token transfer must not scan every device at registration time.
-- Non-unique: Telegram explicitly supports shared tokens for other_uids.
-- This migration changes no registrations or auth/session bindings.
-- Requires 0_migrate_helpers.sql (installed by deploy/migrate.sh). The helper
-- makes sequential re-runs a no-op by index name; verify any pre-existing index
-- with this name has the expected (token_type, token) definition.
-- On InnoDB/MySQL 8 this is an online secondary-index build, not a table copy.
-- LOCK=NONE fails rather than falling back to a blocking algorithm. A short
-- metadata lock and disk/IO for the new index are still required; deploy with
-- the existing migration/deploy lock, a backup and adequate free disk.
CALL teamgram_add_index('devices', 'idx_devices_push_token',
 'ALTER TABLE devices ADD INDEX idx_devices_push_token (token_type, token), ALGORITHM=INPLACE, LOCK=NONE');
