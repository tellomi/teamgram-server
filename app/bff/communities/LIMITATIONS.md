# Communities

Layer 232 `communities.*` methods are routed on `RPCCommunities`.

Communities are stored in `communities`, `community_links`, and
`community_members` (`teamgramd/deploy/sql/migrate-20261114-communities.sql`).
The bff opens those tables with `mysqltls.Open` from `ChannelMySQL` or
`TEAMGRAM_MYSQL_DSN`. With no DSN the book stays in memory. A configured DSN
that cannot be opened does not fall back to memory: community RPCs return
`INTERNAL_SERVER_ERROR` and the next call tries the database again.

`communities.getJoinedCommunities` lists a community for its owner, for a
user peer that was linked and is visible, for someone whose link request was
approved, and for a member of a linked channel. There is no invite RPC in
this batch. A basic-group link does not by itself add every group member;
this process does not load that member list.

`communities.create` and `communities.toggleCommunityCollapsedInDialogs`
return `updates` with seq 0, the `community` chat, and `updateChannel`.
`updateChannel` has no pts, so the client's difference cursor does not move.

Linked chats are stored as peer ids. `communities.participantJoinedChats`
returns those ids. Its `chats` and `users` vectors stay empty because this
process does not load foreign chat objects.

## Schema-valid results

- `communities.create` returns the updates box above. `hidden` stores the
  first peer link as not visible. `about` is kept with the community. This
  batch has no full-chat RPC, so clients do not read `about` back.
- `communities.getJoinedCommunities` returns `messages.chats`.
- `communities.togglePeerLink` returns `boolTrue`. The owner applies
  `visible`, `hidden`, or `deleted` immediately. Another user files a pending
  request instead.
- `communities.getPeerLinkRequests` returns `communities.peerLinkRequests`.
  `limit` 0 means 20. A further page sets `next_offset`.
- `communities.togglePeerLinkRequestApproval` and
  `communities.toggleAllPeerLinkRequestApproval` return `boolTrue`.
  `reject` drops the request. Otherwise the link becomes approved and keeps
  the visibility the requester asked for.
- `communities.toggleCommunityCollapsedInDialogs` returns the updates box
  with `collapsed_in_dialogs` set.
- `communities.toggleParticipantBanned` returns `boolTrue`.
- `communities.getParticipantJoinedChats` returns
  `communities.participantJoinedChats`. `creator_chat_ids` are approved chat
  or channel links that participant requested. `joined_chat_ids` are the
  other approved chat or channel links when that participant is a member.

## Documented 400s

| Code | When |
| --- | --- |
| `CHAT_TITLE_EMPTY` | title is empty or only spaces |
| `TITLE_INVALID` | title is longer than 128 characters |
| `ABOUT_TOO_LONG` | about is longer than 255 UTF-16 code units |
| `PEER_ID_INVALID` | peer is missing, empty, or not a user, chat, or channel. Also approving a peer that has no pending request |
| `PEER_LINK_INVALID` | `communities.togglePeerLink` sets none of visible, hidden, or deleted |
| `CHANNEL_INVALID` | community is missing, `inputChannelEmpty`, or the access hash does not match |
| `CHANNEL_PRIVATE` | collapse is called by someone who did not create the community, or joined-chats is read by someone who is neither the owner nor the participant |
| `CHAT_ADMIN_REQUIRED` | a non-owner deletes a link, reads requests, approves requests, or bans a participant. A banned user filing a new link request gets the same code |
| `USER_ID_INVALID` | ban or joined-chats participant is not a user, or the user id is 0 |
| `USER_CREATOR` | the owner is the ban target |
| `USER_BANNED_IN_CHANNEL` | a banned participant reads their own joined chats |
| `LIMIT_INVALID` | request `limit` is negative or greater than 100 |
| `OFFSET_INVALID` | request `offset` is not empty and not a non-negative integer |

`AUTH_KEY_UNREGISTERED` (401) is returned when the RPC has no user id.
