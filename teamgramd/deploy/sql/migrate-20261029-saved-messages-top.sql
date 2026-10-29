-- Saved Messages dialogs are user_id = peer_id (peer_type 2, or peer_type 1
-- when stored as inputPeerSelf). Older sends wrote the messages and left
-- top_message at 0, so the dialog has no latest message and no date.
--
-- Point each of those dialogs at its newest non-deleted message. A message
-- stored as the other self peer type still counts. Other people's dialogs
-- are not touched. Running this again changes nothing.

UPDATE dialogs d
INNER JOIN (
  SELECT m.user_id, m.user_message_box_id AS top_message, m.date2
  FROM messages m
  INNER JOIN (
    SELECT user_id, MAX(user_message_box_id) AS top_message
    FROM messages
    WHERE deleted = 0
      AND peer_id = user_id
      AND peer_type IN (1, 2)
    GROUP BY user_id
  ) newest
    ON newest.user_id = m.user_id
   AND newest.top_message = m.user_message_box_id
  WHERE m.deleted = 0
    AND m.peer_id = m.user_id
    AND m.peer_type IN (1, 2)
) src ON src.user_id = d.user_id
SET d.top_message = src.top_message,
    d.date2 = src.date2
WHERE d.user_id = d.peer_id
  AND d.peer_type IN (1, 2)
  AND (d.top_message <> src.top_message OR d.date2 <> src.date2);
