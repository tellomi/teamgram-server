# Ephemeral

Layer 232 `ephemeral.*` methods are routed on `RPCEphemeral`.

Messages live in this process, keyed by the sender, the peer, and the
receiver. A restart drops them. Nothing is written into the normal message
box, so these calls do not advance pts.

`ephemeral.sendMessage` returns `updates` with seq 0 and
`updateNewEphemeralMessage`. `ephemeral.editMessage` returns `updates` with
seq 0 and `updateEditEphemeralMessage`. Neither update has pts, so the
client's difference cursor does not move. Deletes return `boolTrue`. The
client drops the row locally.

Media and rich messages are refused. Text, entities, reply markup, and a
reply to a message id are stored. `ephemeral.getCallbackAnswer` records the
click and returns `messages.botCallbackAnswer` with no alert and no message.
No bot is called.

## Schema-valid results

- `ephemeral.sendMessage` stores the row. `welcome` marks a welcome template.
  `anchor` sets `anchor_msg_id` from the reply. The same `random_id` returns
  the original updates.
- `ephemeral.editMessage` returns the edited row. `message` is a flag; when
  it is omitted the previous text stays. `welcome` and `invert_media` follow
  the bools on the request, so an edit that omits them clears those flags.
- `ephemeral.deleteMessage` removes one row addressed to `receiver_id`.
- `ephemeral.deleteWelcomeMessage` removes one welcome template in the peer.
- `ephemeral.deleteAllWelcomeMessages` removes every welcome template in the
  peer and returns `boolTrue` when there were none.
- `ephemeral.getWelcomeMessages` returns `ephemeral.welcomeMessages`. A
  matching non-zero hash returns `ephemeral.welcomeMessagesNotModified`.
- `ephemeral.getCallbackAnswer` returns `messages.botCallbackAnswer`.
- `ephemeral.reportMessage` returns `reportResultChooseOption` when `option`
  is empty, `reportResultAddComment` for `other` without a comment, and
  `reportResultReported` for a known option.

## Documented 400s

| Code | When |
| --- | --- |
| `PEER_ID_INVALID` | peer is missing, empty, or not a user, chat, or channel |
| `USER_ID_INVALID` | `receiver_id` is missing, empty, or has user id 0 |
| `RANDOM_ID_EMPTY` | `ephemeral.sendMessage` `random_id` is 0 |
| `MESSAGE_EMPTY` | send text is empty, or an edit sets the message to empty |
| `MESSAGE_TOO_LONG` | text is longer than 4096 UTF-16 code units, or a report comment is longer than 1024 bytes |
| `ENTITIES_TOO_LONG` | more than 100 entities |
| `ENTITY_BOUNDS_INVALID` | an entity is empty or outside the text |
| `MEDIA_INVALID` | `media` is set and is not `inputMediaEmpty` |
| `RICH_MESSAGE_UNSUPPORTED` | `rich_message` is set |
| `REPLY_TO_INVALID` | `reply_to` is not `inputReplyToMessage` or `inputReplyToEphemeralMessage`, or its id is 0 |
| `ANCHOR_INVALID` | `anchor` is set and there is no reply |
| `EPHEMERAL_TOO_MUCH` | the peer already has 100 ephemeral messages from this sender |
| `MESSAGE_ID_INVALID` | edit, delete, callback, or report names a missing id, or delete-welcome names a message that is not a welcome template |
| `DATA_INVALID` | callback `data` is empty |
| `OPTION_INVALID` | report option is not one of the choices |

`AUTH_KEY_UNREGISTERED` (401) is returned when the RPC has no user id.
