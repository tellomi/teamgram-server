-- Drop self-read receipts left by messages.readHistory.
--
-- When a user read up to their own latest message, the server called
-- inbox.readOutboxHistory with that user as both the outbox owner and the
-- peer. That inserted message_read_outbox rows with user_id = read_user_id,
-- created an empty dialogs row with user_id = peer_id, and set
-- read_outbox_max_id on the real dialog (and on Saved Messages) as if someone
-- else had read the messages.
--
-- Saved Messages is a real dialog: peer_type 2 (inputPeerSelf is stored as a
-- user peer) or peer_type 1, with user_id = peer_id, and it has messages, a
-- top message, a pin, or a draft. Those rows stay. Empty user_id = peer_id
-- rows with none of that are the fake dialogs and are deleted.
--
-- read_outbox_max_id is lowered to the newest own message that another
-- participant's read_inbox_max_id actually covers, or to a message_read_outbox
-- row written for a different reader. It is never raised. Running this file
-- again changes nothing.

-- peer_type 1 is Saved Messages stored as inputPeerSelf. Keep it.
-- peer_type 2 with user_id = peer_id is Saved Messages stored as a user peer
-- once it has history. Anything else with that key and no history is the
-- dialog inbox.readOutboxHistory inserted for the reader themselves.
DELETE d FROM dialogs d
WHERE d.user_id = d.peer_id
  AND d.peer_type = 2
  AND d.top_message = 0
  AND d.pinned = 0
  AND d.folder_pinned = 0
  AND d.draft_type = 0
  AND NOT EXISTS (
    SELECT 1 FROM messages m
    WHERE m.user_id = d.user_id
      AND m.peer_id = d.user_id
      AND m.peer_type IN (1, 2)
  );

-- Nobody else reads Saved Messages. A non-zero marker here came from the
-- self-read bug (often a message id from a different chat).
UPDATE dialogs
SET read_outbox_max_id = 0
WHERE user_id = peer_id
  AND peer_type IN (1, 2)
  AND read_outbox_max_id <> 0;

DELETE FROM message_read_outbox
WHERE user_id = read_user_id;

-- Private dialogs. proven is the sender's box id of messages the peer has
-- read. logged is the same fact recorded in message_read_outbox. Self rows
-- were deleted above, so logged cannot replay the bug.
UPDATE dialogs d
LEFT JOIN (
  SELECT mine.user_id, mine.peer_id, MAX(mine.user_message_box_id) AS read_outbox_max_id
  FROM messages mine
  INNER JOIN messages theirs
    ON theirs.dialog_message_id = mine.dialog_message_id
   AND theirs.dialog_message_id <> 0
   AND theirs.user_id = mine.peer_id
   AND theirs.user_id <> mine.user_id
   AND theirs.deleted = 0
  INNER JOIN dialogs reader
    ON reader.user_id = theirs.user_id
   AND reader.peer_type = 2
   AND reader.peer_id = mine.user_id
   AND reader.read_inbox_max_id >= theirs.user_message_box_id
  WHERE mine.peer_type = 2
    AND mine.user_id <> mine.peer_id
    AND mine.sender_user_id = mine.user_id
    AND mine.deleted = 0
  GROUP BY mine.user_id, mine.peer_id
) proven ON proven.user_id = d.user_id AND proven.peer_id = d.peer_id
LEFT JOIN (
  SELECT user_id, peer_dialog_id AS peer_id, MAX(read_outbox_max_id) AS read_outbox_max_id
  FROM message_read_outbox
  WHERE user_id <> read_user_id
  GROUP BY user_id, peer_dialog_id
) logged ON logged.user_id = d.user_id AND logged.peer_id = d.peer_id
SET d.read_outbox_max_id = GREATEST(
  COALESCE(proven.read_outbox_max_id, 0),
  COALESCE(logged.read_outbox_max_id, 0)
)
WHERE d.peer_type = 2
  AND d.user_id <> d.peer_id
  AND d.read_outbox_max_id > GREATEST(
    COALESCE(proven.read_outbox_max_id, 0),
    COALESCE(logged.read_outbox_max_id, 0)
  );

-- Basic groups. The receipt table is not written for chats, so the other
-- participants' read_inbox_max_id is the source of truth.
UPDATE dialogs d
LEFT JOIN (
  SELECT mine.user_id, mine.peer_id AS chat_id, MAX(mine.user_message_box_id) AS read_outbox_max_id
  FROM messages mine
  INNER JOIN messages theirs
    ON theirs.dialog_message_id = mine.dialog_message_id
   AND theirs.dialog_message_id <> 0
   AND theirs.user_id <> mine.user_id
   AND theirs.peer_type = 3
   AND theirs.peer_id = mine.peer_id
   AND theirs.deleted = 0
  INNER JOIN dialogs reader
    ON reader.user_id = theirs.user_id
   AND reader.peer_type = 3
   AND reader.peer_id = mine.peer_id
   AND reader.read_inbox_max_id >= theirs.user_message_box_id
  WHERE mine.peer_type = 3
    AND mine.sender_user_id = mine.user_id
    AND mine.deleted = 0
  GROUP BY mine.user_id, mine.peer_id
) proven ON proven.user_id = d.user_id AND proven.chat_id = d.peer_id
SET d.read_outbox_max_id = COALESCE(proven.read_outbox_max_id, 0)
WHERE d.peer_type = 3
  AND d.read_outbox_max_id > COALESCE(proven.read_outbox_max_id, 0);
