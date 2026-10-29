# Voice and video calls

Stock Telegram clients (API layer 232) complete a 1:1 call without the server
seeing media. The server exchanges Diffie-Hellman public values, relays
`phone.sendSignalingData`, and hands out connection endpoints. Group voice
chats additionally need an SFU that speaks the tgcalls join JSON.

## What is implemented

- `messages.getDhConfig` returns the 2048-bit prime (generator 3) clients use
  to compute `g_a` / `g_b`. `phone.requestCall`, `acceptCall`, and `confirmCall`
  check `SHA256(g_a)` and reject public values outside the safe subgroup.
- `updatePhoneCall` / `updatePhoneCallSignalingData` and the `phoneCall*`
  constructors, including per-user `phoneConnection` (UDP reflector peer tag)
  and `phoneConnectionWebrtc` (coturn REST credentials).
- Ring timeout (default 90s) discards with `phoneCallDiscardReasonMissed`.
  Confirm timeout (default 30s) discards with `phoneCallDiscardReasonDisconnect`.
- Group calls: create, join, leave, discard, participants, mute and hand-raise,
  title, join-muted and messages flags, invite link, recording flags, scheduled
  start, presentation join/leave, stream-channel descriptor, and RTMP url/key.
- A process-local SFU (`app/bff/phone/internal/sfu`) answers with the JSON shape
  from Telegram's `tgcalls/tools/go_sfu`: `transport.fingerprints[].setup` is
  `"active"` (SFU is the DTLS client; tgcalls is the DTLS server). Audio RTP is
  forwarded with the sender's SSRC. coturn and the reflector are in
  `docker-compose.yaml`.

`Phone.PublicIP` (or `TELLOMI_CALL_IP`) is the address written into ICE
candidates and phone connections. It has to be reachable from the handset.
coturn's `external-ip` in `teamgramd/deploy/coturn/turnserver.conf` must be the
same address, and `static-auth-secret` must match `Phone.TurnSecret`.

## What is not equivalent to Telegram

- Conference calls (`phone.createConferenceCall` and the E2EE blockchain) are
  not implemented. A `block` sent on `phone.joinGroupCall` is stored and pushed
  as `updateGroupCallChainBlocks`, which is not a full chain.
- The SFU does not speak tgcalls colibri (simulcast layer selection, receiver
  video constraints, or transport-cc feedback). Video payload types are H264
  baseline in the join answer, and video RTP is forwarded on the same socket as
  audio. Screen share has a second DTLS association only when the client opens
  one; there is no separate quality ladder.
- `phone.toggleGroupCallRecord` sets `record_start_date` and
  `record_video_active` and counts decrypted RTP packets. It does not write an
  audio file or a playable recording.
- `phone.getGroupCallStreamChannels` returns a channel descriptor.
  `upload.getFile` / `inputGroupCallStream` segment download is not
  implemented, so in-app live viewers do not receive media.
- `phone.getGroupCallStreamRtmpUrl` returns a URL aimed at the MediaMTX
  container. MediaMTX accepts the RTMP push. That ingest is not restreamed into
  the SFU, so the group call does not show the RTMP stream.
- `phone.saveCallLog` acknowledges the upload and does not store the file.
- Call state is in memory in the bff process. The `phone_peer_call` table only
  publishes which chat has a live group call, so `chat.call` / `channelFull.call`
  survive a second process. A second bff instance will not see the call itself.
- Basic-group admin checks use the biz chat service when it is configured.
  Without that client, the caller is treated as allowed to start the call.
