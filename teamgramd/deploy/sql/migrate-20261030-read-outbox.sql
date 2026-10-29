-- Recompute read_outbox_max_id from receipts another participant actually has.
--
-- Sending a message, and the sender's own messages.readHistory / channels.readHistory
-- (iOS does this while the chat is open), had been stored as if someone else had
-- read it. Supergroups wrote that into channel_read_state. Private chats and basic
-- groups wrote it into dialogs. This lowers those cursors to the newest own message
-- that a different participant's read_inbox_max_id covers. It never raises them.
--
-- A snapshot of the cursors is taken first. Running this file again does not
-- overwrite that snapshot and does not change a cursor that is already correct.

CREATE TABLE IF NOT EXISTS dialogs_read_outbox_bak_20261030 (
  user_id BIGINT NOT NULL,
  peer_type INT NOT NULL,
  peer_id BIGINT NOT NULL,
  top_message INT NOT NULL,
  read_inbox_max_id INT NOT NULL,
  read_outbox_max_id INT NOT NULL,
  backed_up_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, peer_type, peer_id)
);

INSERT IGNORE INTO dialogs_read_outbox_bak_20261030
  (user_id, peer_type, peer_id, top_message, read_inbox_max_id, read_outbox_max_id)
SELECT user_id, peer_type, peer_id, top_message, read_inbox_max_id, read_outbox_max_id
FROM dialogs;

CREATE TABLE IF NOT EXISTS channel_read_state_bak_20261030 (
  channel_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  read_inbox_max_id INT NOT NULL,
  read_outbox_max_id INT NOT NULL,
  unread_count INT NOT NULL,
  backed_up_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (channel_id, user_id)
);

INSERT IGNORE INTO channel_read_state_bak_20261030
  (channel_id, user_id, read_inbox_max_id, read_outbox_max_id, unread_count)
SELECT channel_id, user_id, read_inbox_max_id, read_outbox_max_id, unread_count
FROM channel_read_state;

-- Nobody else reads Saved Messages.
UPDATE dialogs
SET read_outbox_max_id = 0
WHERE user_id = peer_id
  AND peer_type IN (1, 2)
  AND read_outbox_max_id <> 0;

DELETE FROM message_read_outbox
WHERE user_id = read_user_id;

-- Private dialogs. proven is the sender's box id of messages the peer has read.
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

-- Basic groups. Another participant's read_inbox_max_id is the receipt.
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

-- Supergroups and channels. The sender's own read_inbox_max_id does not count.
UPDATE channel_read_state s
LEFT JOIN (
  SELECT m.sender_user_id AS user_id, m.channel_id, MAX(m.message_id) AS read_outbox_max_id
  FROM channel_messages m
  INNER JOIN channel_read_state r
    ON r.channel_id = m.channel_id
   AND r.user_id <> m.sender_user_id
   AND r.read_inbox_max_id >= m.message_id
  WHERE m.deleted = 0
    AND m.sender_user_id <> 0
  GROUP BY m.sender_user_id, m.channel_id
) proven ON proven.user_id = s.user_id AND proven.channel_id = s.channel_id
SET s.read_outbox_max_id = COALESCE(proven.read_outbox_max_id, 0)
WHERE s.read_outbox_max_id > COALESCE(proven.read_outbox_max_id, 0);

-- Shadow rows in dialogs for the same channels. Only lower them to the
-- recomputed channel_read_state value.
UPDATE dialogs d
LEFT JOIN channel_read_state s
  ON s.user_id = d.user_id
 AND s.channel_id = d.peer_id
SET d.read_outbox_max_id = COALESCE(s.read_outbox_max_id, 0)
WHERE d.peer_type = 4
  AND d.read_outbox_max_id > COALESCE(s.read_outbox_max_id, 0);
