-- Private-chat inbox rows used to store messages.peer_id as the box owner
-- (the recipient) instead of the other user. getHistory, dialogs and most
-- deletes already key on dialog_id1/dialog_id2, so those paths keep
-- working. This updates the column in place so peer-scoped readers
-- (TTL backfill, report, wipe, media grouping) match the outbox and the
-- dialogs row. Nothing is deleted. Safe to run more than once.

UPDATE messages
SET peer_id = sender_user_id
WHERE peer_type = 2
  AND peer_id = user_id
  AND sender_user_id <> 0
  AND sender_user_id <> user_id;
