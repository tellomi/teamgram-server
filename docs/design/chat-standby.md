# Chat primary and standby (M8 / D-70)

Design only. This page does not add a process, a flag, a migration, or a compose service. Do not treat it as a runbook that has been rehearsed. D-70 says the chat core stays in Chengdu until this scheme is built, and that the scheme is not built yet.

Chat is Teamgram over MTProto. Clients keep a TCP connection open. A platform that starts a container per request and stops it when idle cannot hold those connections, so chat does not move to Cloud Run. The standby is a second machine that can be promoted by hand.

## What has to move together

A promoted standby is a whole chat stack, not a MySQL replica with the Chengdu processes still attached.

| State | Where it lives | What a switch must do |
| --- | --- | --- |
| Users, dialogs, messages, contacts | MySQL | Replicate, then promote. The replica is read-only until then. |
| Login keys | MySQL `auth_keys`, `auth_key_infos`, `auth_users` | Same replica. A client keeps its auth key and does not sign in again. |
| Cloud password | MySQL `user_passwords`, `user_password_srp`, `user_password_login` | Same replica. |
| Message and pts counters | Redis keys below, with a MySQL floor | Rebuild on the standby. Do not let them go backwards. |
| Secret-chat qts | Redis only | Cannot be rebuilt. See below. |
| Updates still in the broker | Kafka | Drain or accept a loss of what was not yet in MySQL. |
| Service discovery | etcd | New cluster on the standby. Do not copy Chengdu's etcd. |
| Photos, files, stickers | Cloudflare R2 | Already outside Chengdu. No copy step. |

`idgen` stores these Redis keys (`app/service/idgen/internal/seqfloor/counter.go`):

| Prefix | Rebuilt from |
| --- | --- |
| `message_box_ngen_` | private message box ids |
| `channel_message_box_ngen_` | `channel_messages` and `channels.top_message` |
| `pts_updates_ngen_` | `user_pts_updates` |
| `channel_pts_updates_ngen_` | channel pts |
| `seq_updates_ngen_` | `auth_seq_updates` |
| `story_ngen_`, `channel_story_ngen_` | `stories`, when that table exists |
| `qts_updates_ngen_` | nothing. Secret-chat qts is not in MySQL. |

`seqfloor` already refuses to hand out an id below the MySQL maximum. On startup, `Heal` raises a missing or low Redis key up to that floor and never lowers a key that is already higher. `FLUSHALL` is still forbidden: it drops the keys, and qts cannot be repaired afterwards. The backup job copies `dump.rdb` and `appendonlydir` and does not send `FLUSH`.

## Standby machine

One spare host, in Hong Kong, Singapore, or another mainland region. Same compose files as Chengdu. It does not take client traffic while Chengdu is primary.

The spare MySQL is a replica. The spare Redis starts empty at promotion time, or is whatever `Heal` repairs. The spare etcd is its own cluster. Copying Chengdu's etcd would advertise Chengdu addresses to the spare processes.

## MySQL

Use asynchronous replication from Chengdu to the spare. The spare stays `read_only` (and `super_read_only` if the server version has it). App users on the spare must not be able to write.

Login survives promotion because the auth key body and the `auth_users` row that binds it to a user id are in the replicated tables. Password verifiers are in the same database. Sessions that were only in the Chengdu process memory (in-flight RPC, online status in Redis) drop. The next connection uses the same auth key.

Stop writers before promotion. A replica that is seconds behind, promoted while Chengdu is still accepting sends, splits the history: Chengdu has rows the spare will never see, and clients that already applied those pts will not accept a lower pts.

## Redis counters

Duplicates are worse than gaps. A repeated message-box id or pts attaches a new update to an old row. A gap makes a client call `updates.getDifference` and move on.

Promotion rules:

1. Chengdu teamgram is stopped, so it is no longer allocating ids.
2. The replica has applied the last Chengdu commit.
3. The spare Redis is empty, or its counters are healed against the promoted database.
4. `idgen` runs `Heal` before `bff` accepts connections. That is what a normal process start already does when `Mysql.DSN` is set.

`Heal` covers the prefixes in the table above. It does not cover `qts_updates_ngen_*`. After a Redis loss, secret-chat qts can repeat or skip. Ordinary cloud chats do not use qts. The manual switch accepts that secret-chat gap rather than copying a live Redis that might be ahead of a lagging replica. Do not restore an old `dump.rdb` onto the spare after promotion: an old dump is a rewind, and `Heal` only raises keys that are below MySQL, not qts.

A Redis replica is not required for this design. If one is added later, it is still not the source of truth. `Heal` against the promoted MySQL remains the step that prevents a rewind of the counters that have a table.

## Kafka and etcd

`msg` and `sync` publish updates through Kafka and then store the result MySQL already has, or is about to have. A clean switch stops `teamgram` on Chengdu and lets the consumer finish what is already in the log before MySQL promotion. What was still only in Kafka, and not in MySQL, is gone. Clients recover updates that did reach MySQL through `updates.getDifference`. This design does not add a Kafka mirror.

etcd on the spare is empty at first. Processes register themselves when they start. Do not restore Chengdu's snapshot onto it.

## Client addresses

`TELLOMI_DC_OPTIONS` already replaces `help.getConfig` `dc_options` with a list of `dcId:ipv4:port`. DC 1 may have more than one address. Clients that understand a DC with several IPs try the next address when the first does not connect. Android only accepts IPv4 (`inet_pton`). Both `10443` and `5222` are listed today, with `this_port_only`. See [DC_ADDRESSES.md](../ops/DC_ADDRESSES.md).

The spare IP stays out of that list until the spare is promoted and `teamgram` is listening. A client that dials a read-only replica would fail writes in a way that looks like a server error, and might stick to that address. Connection refused, from an IP where nothing is listening, is the failure mode the client already treats as "try the next IP".

Two ways a client learns the spare, both required before the first real outage:

1. The installed client has both IPs. Chengdu is first, the spare is second. That seed is a client change, in the iOS and Android repos, not in this one. Until that ships, a dead Chengdu cannot push a new `dc_options` list.
2. After promotion, Chengdu's replacement serves `TELLOMI_DC_OPTIONS` with the spare's IPs only. Clients that can still reach a live server pick the new list up from `help.getConfig` (D-33). Keeping the dead Chengdu IP in the list makes every cold start wait on a timeout.

Manual switch, in order:

1. Confirm the replica is connected and the relay log is caught up.
2. Stop the Chengdu `teamgram` container so clients and Kafka producers stop.
3. Let the replica apply the last events. Do not promote while `Seconds_Behind_Source` is still moving.
4. Promote the replica and clear the replica configuration so a restart does not resume replication from Chengdu.
5. Start spare Redis empty. Start etcd empty. Start `teamgram` so `idgen` heals counters before `gnetway` is published.
6. Set `TELLOMI_DC_OPTIONS` on the spare to its own IPs and ports. Recreate the container so the process sees the variable.
7. Leave Chengdu MySQL as a spare only after it has been rebuilt as a replica of the new primary. Do not start Chengdu `teamgram` against the old database.

There is no automatic promotion in this design. A later version can add a health check and a single operator command. It still must not run two writable MySQL primaries.

## Memory budget (D-69)

No cap in `docker-compose.limits.yaml` changes with this document. The 3584m teamgram cap stays. Chengdu already hit a full CPU in reclaim at 2560m, and clients sat on "connecting".

The opt-in overlay (`TELLOMI_COMPOSE_LIMITS=1`) is a ceiling, not a reservation. On a host with about 7.1 GiB free for containers, the ceilings are:

| Service | Ceiling |
| --- | --- |
| teamgram | 3584m |
| mysql | 1536m |
| kafka | 1.6g |
| redis | 512m |
| etcd | 256m |
| coturn | 256m |
| mediamtx | 256m |
| asr | 1536m |
| **sum** | **about 9.3 GiB** |

They do not all sit on the ceiling at once. Redis still has no eviction policy, so a 512m kill drops idgen and pts keys. Push and `@tellomi` run inside the `teamgram` container. They do not have their own cap.

Postgres for content-engine and the engine process are not in this compose file. Adding them on the same 8 GB host, on top of the table above, does not fit. D-69's recount is: do not lower teamgram, do not put the feed database on this overlay until something else moves off the host (asr is the large optional piece, at 1536m), and do not turn the overlay on until the host's current RSS is measured again. This page does not change those numbers.

## What else in M8 is not this change

| Decision | Where it stands for this server |
| --- | --- |
| D-62 help site on Cloudflare Pages | Not this repository. `faq_link` is still `https://tellomi.cn/faq` until a later change. |
| D-63 R2 in front of a CDN | `TELLOMI_S3_PUBLIC_MEDIA_BASE_URL` and `TELLOMI_S3_ATTACHMENTS_BASE_URL` already exist. Binding the hostname is Cloudflare configuration. See [storage.md](../storage.md). |
| D-64, D-65, D-66 feed split, Supabase replica, offline queue | content-engine and the clients. Not this process. |
| D-67 `@tellomi` on Cloud Run | The server path is in-process and off unless `TELLOMI_AI=1`. The daily counter is per process. A second region cannot share it until that counter moves to a store both sides read. Not built here. |
| D-68 push | APNs is sent when the `.p8` is mounted. Xiaomi is sent only when `TELLOMI_XIAOMI_PUSH=1` (token type 14). FCM and Huawei tokens are stored and not sent. Other Android brands have no channel. Whether Chengdu can reach FCM was not measured from this repo. See [push.md](../push.md). |
| D-69 alerts | Still no monitor and no pager. The memory note above is the recount. |
| D-71 privacy text | The public privacy page, not this server. |

## Out of scope

- A second writable primary.
- Automatic failover.
- Copying Redis with `FLUSHALL` or restoring an older dump over a newer counter.
- Publishing the spare IP before `teamgram` is listening on the promoted database.
- Moving MTProto to Cloud Run.
