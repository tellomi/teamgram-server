-- Optional and idempotent. Do not run this from startup, CI, or a deploy hook.
--
-- Accounts with no user_presences row, and placeholder rows inserted with
-- last_seen_at = 0, used to be readable as unix 0 (1970-01-01). The server
-- now renders a missing or zero timestamp as userStatusEmpty. This script
-- gives those accounts the registration time and expires 0, so a direct
-- read of the table is an offline "last seen" at signup rather than 1970.
-- An existing heartbeat (last_seen_at > 0, or expires > 0) is left alone.
-- Running it twice changes nothing the second time.

INSERT INTO user_presences (user_id, last_seen_at, expires)
SELECT u.id, UNIX_TIMESTAMP(u.created_at), 0
FROM users u
LEFT JOIN user_presences p ON p.user_id = u.id
WHERE p.user_id IS NULL
  AND u.deleted = 0
  AND UNIX_TIMESTAMP(u.created_at) > 0;

UPDATE user_presences p
JOIN users u ON u.id = p.user_id
SET p.last_seen_at = UNIX_TIMESTAMP(u.created_at)
WHERE p.last_seen_at = 0
  AND p.expires = 0
  AND u.deleted = 0
  AND UNIX_TIMESTAMP(u.created_at) > 0;
