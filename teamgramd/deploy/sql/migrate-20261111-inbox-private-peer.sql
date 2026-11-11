-- Private-chat inbox rows used to store messages.peer_id as the box owner
-- (the recipient) instead of the other user. getHistory, dialogs and most
-- deletes already key on dialog_id1/dialog_id2, so those paths keep
-- working. This updates the column in place so peer-scoped readers
-- (TTL backfill, report, wipe, media grouping) match the outbox and the
-- dialogs row. Nothing is deleted.
--
-- The procedure lives in 0_migrate_helpers.sql and applies to the
-- unsharded `messages` table and to every `messages_N` shard, if present.
-- Backup tables (messages_bak_*) are not touched. Saved Messages
-- (sender_user_id = user_id) are left unchanged. Safe to run more than
-- once: the UPDATE is a no-op when peer_id is already the sender.

CALL tellomi_fix_inbox_private_peer();
