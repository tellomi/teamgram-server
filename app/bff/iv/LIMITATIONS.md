# iv.getRichMessage

Android `TL_iv.getRichMessage` is constructor `0x501569cf`. That is
`messages.getRichMessage` in the layer-232 schema. The session decodes it as
`messages.getRichMessage` and `RPCMessages` serves it.

`RPCIv` is registered for the coverage name. It runs the same load.

The method returns `messages.channelMessages` for a channel post the caller
can see, including the rich-message body when the stored message has one.
Private chats and basic groups are not loaded here.

## Documented 400s

| Code | When |
| --- | --- |
| `PEER_ID_INVALID` | `peer` is missing |
| `MESSAGE_ID_INVALID` | `id` is not positive, the peer is not a channel, or the channel has no such message |
| `CHANNEL_INVALID` | the channel id does not resolve |
| `CHANNEL_PRIVATE` | the caller cannot view the channel |

`AUTH_KEY_UNREGISTERED` (401) is returned when the RPC has no user id.
