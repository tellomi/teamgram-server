# Stats

`stats.getBroadcastStats` and `stats.getMegagroupStats` were already served.
This batch adds the other `stats.*` methods on `RPCStatistics`.

Graphs are `statsGraphAsync` tokens kept in this process. `stats.loadAsyncGraph`
returns `statsGraph` JSON for a known token. Token `0` is the placeholder
already used by broadcast and megagroup stats and returns an empty line.
A token this process minted and then lost returns `GRAPH_EXPIRED_RELOAD`.

Message public forwards do not store the forwarded copies. `count` is the
forwards counter on the message and `forwards` is empty. Story public
forwards use the story's forward counter the same way.

Poll stats read the channel message when the peer is a channel, then the
poll record for that dialog message when one exists.

## Documented 400s

| Code | When |
| --- | --- |
| `CHANNEL_INVALID` | the channel does not resolve |
| `CHANNEL_PRIVATE` | the caller is not a member |
| `CHAT_ADMIN_REQUIRED` | message stats or message forwards are requested by a non-admin, or story stats are requested for a channel the caller does not manage |
| `MESSAGE_ID_INVALID` | the message id is not positive, the message is missing, or the message is not a poll for `stats.getPollStats` |
| `PEER_ID_INVALID` | the peer is missing or not a user, chat, or channel. Also story stats for another user's stories |
| `STORY_ID_INVALID` | the story id is not positive or that story does not exist |
| `STORIES_NEVER_CREATED` | the peer has no stories |
| `OFFSET_INVALID` | a forwards offset is not empty and not a non-negative integer |
| `LIMIT_INVALID` | `limit` is negative or greater than 100 |
| `GRAPH_INVALID_RELOAD` | the async token is empty or unknown |
| `GRAPH_EXPIRED_RELOAD` | the token was minted here and is no longer stored. Token `expired` returns the same code |
| `GRAPH_OUTDATED_RELOAD` | the token is `outdated` |

`AUTH_KEY_UNREGISTERED` (401) is returned when the RPC has no user id.
