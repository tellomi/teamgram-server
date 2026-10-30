# 1:1 voice and video calls

Stock Telegram clients (API layer 232) finish a 1:1 call without the server
seeing media. The server exchanges Diffie-Hellman values, relays
`phone.sendSignalingData`, and returns STUN/TURN addresses on the confirmed
call. coturn on the Chengdu Docker host (`8.137.39.82`) is the relay those
addresses point at.

Signaling uses the MTProto port the client already has: production TCP
**10443**, staging `staging-api.tellomi.cn` TCP **11443**. TURN is a separate
UDP/TCP listener. Restarting the `teamgram` container picks up the session
route. This does not flush Redis.

## Why calls did not connect

`phone.*` was implemented in `app/bff/phone` and registered on the bff
process, and `teamgramd/etc/session.yaml` left `/mtproto.RPCVoipCalls`
commented out. The session answered `phone.getCallConfig` with `{}` and
every other `phone.*` method with `METHOD_NOT_IMPL`, so the client never
received a `phoneCall` or a relay address.

`messages.getDhConfig` lives on `RPCSecretChats`, which was also commented
out. Android `VoIPService` calls it from `startOutgoingCall` and
`acceptIncomingCall`. iOS loads the same prime from
`SecretChatEncryptionConfig` before it can build `g_a`.

Both services are routed to `bff.bff` in `teamgramd/etc/session.yaml` and
`teamgramd/etc2/session.yaml`. `RPCGroupCalls` stays commented out. Group
voice is a later slice. 1:1 video is the same methods with `video:true`.

## What the clients call

| Order | Method | Role |
| --- | --- | --- |
| 1 | `phone.getCallConfig` | Codec, jitter, `enable_p2p`, `force_tcp`. Cached by iOS `ManagedVoipConfigurationUpdates` and Android `VoIPService.updateServerConfig`. STUN and TURN hosts are on `phoneCall.connections`, not in this JSON. |
| 2 | `messages.getDhConfig` | 2048-bit prime and generator 3. The client computes `g_a` / `g_b`. |
| 3 | `phone.requestCall` | Caller sends `g_a_hash` (SHA-256 of `g_a`) and `PhoneCallProtocol`. Callee receives `updatePhoneCall` / `phoneCallRequested`. |
| 4 | `phone.receivedCall` | Callee's device is ringing. |
| 5 | `phone.acceptCall` | Callee sends `g_b`. Caller receives `phoneCallAccepted`. |
| 6 | `phone.confirmCall` | Caller sends `g_a` and `key_fingerprint`. Both sides receive `phoneCall` with `connections` and `g_a_or_b`. |
| 7 | `phone.sendSignalingData` | Opaque WebRTC signaling. The other party gets `updatePhoneCallSignalingData`. |
| 8 | `phone.discardCall` | Hangup, busy, or decline. `phone.setCallRating` and `phone.saveCallDebug` follow. |

`messages.receivedQueue` is on the same gRPC service as `messages.getDhConfig`.
iOS calls it from `AccountStateManager.reset`. The handler returns an empty
vector and does not touch Redis or MySQL. Secret-chat encryption
(`messages.sendEncrypted` and the rest) is still unimplemented and returns
`METHOD_NOT_IMPL`.

## How a client learns STUN and TURN

After `phone.confirmCall`, `phoneCall.connections` contains:

- `phoneConnection` — legacy UDP reflector (`TELLOMI_CALL_IP` and
  `ReflectorPort`, default 3479) plus a 16-byte peer tag. libtgvoip uses this.
  Current tgcalls prefers the WebRTC entries.
- `phoneConnectionWebrtc` with `turn:true` — coturn. `username` is
  `<unix-expiry>:<user id>`, `password` is
  `base64(HMAC-SHA1(secret, username))`. That is coturn `use-auth-secret`.
- `phoneConnectionWebrtc` with `stun:true` — the STUN listener, usually the
  same host and port as TURN.

`ipv6` is set only when `TELLOMI_CALL_IPV6` (or `PublicIPv6`) is an IPv6
address. An IPv4 string is left empty so the client does not treat it as an
IPv6 candidate.

`phoneCall.p2p_allowed` follows `EnableP2P` and the callee's phone-P2P
privacy rule. `phone.getCallConfig` uses the same `enable_p2p` flag, and
`TELLOMI_CALL_FORCE_TCP=true` flips `force_tcp` in that JSON.

## Environment

Yaml defaults live in `teamgramd/etc2/bff.yaml` (`Phone:`). The `teamgram`
service in `docker-compose.yaml` passes these through. An empty variable
keeps the yaml value.

| Variable | Default | Purpose |
| --- | --- | --- |
| `TELLOMI_CALL_IP` | `127.0.0.1` | Public address in `phoneConnection` and the reflector. |
| `TELLOMI_CALL_IPV6` | empty | Written into `ipv6` when it parses as IPv6. |
| `TELLOMI_TURN_HOST` | `TELLOMI_CALL_IP` | Single TURN host when `TELLOMI_TURN_SERVERS` is empty. |
| `TELLOMI_TURN_PORT` | `3478` | TURN port. `0` omits TURN entries. |
| `TELLOMI_STUN_HOST` | TURN host | Single STUN host when `TELLOMI_STUN_SERVERS` is empty. |
| `TELLOMI_STUN_PORT` | `3478` | STUN port. `0` omits STUN entries. |
| `TELLOMI_TURN_SECRET` | `tellomi-turn-secret` | HMAC secret. Must equal coturn `static-auth-secret`. |
| `TELLOMI_TURN_TTL` | `86400` | Lifetime baked into the TURN username. |
| `TELLOMI_TURN_SERVERS` | empty | Comma-separated `host:port`. Replaces the single TURN host. IPv6 is `[2001:db8::1]:3478`. |
| `TELLOMI_STUN_SERVERS` | empty | Same shape, for STUN. |
| `TELLOMI_REFLECTOR_PORT` | `3479` | Legacy reflector. `0` omits `phoneConnection`. |
| `TELLOMI_ENABLE_P2P` | `true` | `phoneCall.p2p_allowed` and `enable_p2p` in getCallConfig. |
| `TELLOMI_CALL_FORCE_TCP` | `false` | `force_tcp` in getCallConfig. |

Set a real `TELLOMI_TURN_SECRET` on the server. The yaml default is for local
compose only.

## coturn on 8.137.39.82

The `coturn` service uses `network_mode: host` and
`teamgramd/deploy/coturn/entrypoint.sh`. The image stays
`coturn/coturn:4.6.2` (it is not built in CI). On the Chengdu host, in the
compose `.env`:

```bash
TELLOMI_CALL_IP=8.137.39.82
TELLOMI_TURN_HOST=8.137.39.82
TELLOMI_TURN_SECRET=<long random secret>
TELLOMI_TURN_PORT=3478
TELLOMI_STUN_PORT=3478
# Omit TURN_EXTERNAL_IP to reuse TELLOMI_CALL_IP.
# Set TURN_RELAY_IP when 8.137.39.82 is an EIP and the NIC has a private address:
# TELLOMI_TURN_RELAY_IP=172.x.x.x
TELLOMI_TURN_MIN_PORT=49160
TELLOMI_TURN_MAX_PORT=49200
```

`ip -4 addr` shows the address on the NIC. If that address is private and
`8.137.39.82` is only the elastic IP, set `TELLOMI_TURN_RELAY_IP` to the
private address. The entrypoint then writes
`external-ip=8.137.39.82/<private>` so relay candidates advertise the public
address. If the public address is configured on the NIC, leave
`TELLOMI_TURN_RELAY_IP` empty.

Apply and restart:

```bash
docker compose -f docker-compose-env.yaml -f docker-compose.yaml up -d coturn teamgram
docker compose logs coturn --tail=50
```

The log line should include `external-ip` of `8.137.39.82` and listener
`3478`. Confirm the secret matches by placing a call and checking that
`phoneCall.connections` username/password are accepted (a 401 in the coturn
log means the secrets differ).

### Ports to open

Aliyun security group **and** the host firewall, inbound, on `8.137.39.82`:

| Port | Protocol | Why |
| --- | --- | --- |
| 3478 | UDP and TCP | STUN and TURN. TCP is the fallback when UDP is filtered. |
| 5349 | TCP | TURN over TLS. Optional. Set `TELLOMI_TURN_TLS_PORT=0` to skip it. A certificate is not configured; clients use 3478. |
| 49160–49200 | UDP | coturn relay range (`TELLOMI_TURN_MIN_PORT` / `MAX_PORT`). Media flows here after TURN allocates. |
| 3479 | UDP | Legacy `phoneConnection` reflector, published by the `teamgram` container. |
| 3480 | UDP | Group-call SFU. Not required for 1:1. |

Leave MTProto where it is. Production stays on TCP 10443 and staging on TCP 11443.
Those ports carry `phone.*`. Media uses the TURN ports above.

Outbound from the host must be allowed so coturn can answer phones. Relay
ports are inbound to the server, not a wide-open outbound policy.

`firewalld` example (skip if the security group is the only filter):

```bash
firewall-cmd --permanent --add-port=3478/udp --add-port=3478/tcp
firewall-cmd --permanent --add-port=49160-49200/udp
firewall-cmd --permanent --add-port=3479/udp
firewall-cmd --reload
```

### What the handset should see

1. `phone.getCallConfig` returns JSON with `audio_frame_size` and
   `enable_p2p`. `{}` means the session process is still running the old
   IDMap. Restart `teamgram`.
2. `messages.getDhConfig` returns a 256-byte prime, or
   `dhConfigNotModified` when the client sends version `3`.
3. After the callee accepts, `phone.confirmCall` returns `phoneCall` whose
   `connections` include `ip: "8.137.39.82"` and port `3478`, with a non-empty
   TURN username and password.

## What still blocks media

Signaling can succeed and the call can still have no audio.

- **coturn not reachable.** Until `TELLOMI_CALL_IP` is `8.137.39.82` and the
  security group allows UDP 3478 plus 49160–49200, ICE candidates point at
  `127.0.0.1` or a closed port. The client reports "connecting" and then
  drops with `phoneCallDiscardReasonDisconnect`.
- **Secret mismatch.** bff and coturn must use the same
  `TELLOMI_TURN_SECRET`. The compose default `tellomi-turn-secret` is only
  for a machine where both containers share that default.
- **UDP filtered on the phone network.** Set `TELLOMI_CALL_FORCE_TCP=true`
  so getCallConfig asks tgcalls to prefer TCP, and keep TCP 3478 open.
  TURN allocations still need the UDP relay range on the server.
- **One bff process.** Call state is in memory. A second bff replica does
  not see the call. The Chengdu compose file runs one `teamgram` container,
  which is one bff. Restarting it drops live calls.
- **Background iOS.** `updatePhoneCall` reaches a connected session. VoIP
  device tokens (`token_type` 9) are stored and are not sent a PushKit
  push, so a suspended iOS app does not get a CallKit ring. Both clients
  in the foreground complete signaling. Wiring the VoIP push is a separate
  change.
- **Group calls.** `RPCGroupCalls` is not routed. The in-process SFU does
  not speak tgcalls colibri. That does not affect 1:1.
