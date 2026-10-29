# Plugging independent services into Tellomi

This is a design note for services that are not Telegram features: swipe matching and a moments feed. Nothing in this document is implemented. It names the seams that already exist so a later change can add a service without folding its data into the chat schema.

## Where a client request goes today

```
client
  → gnetway          MTProto, TCP or WebSocket
  → session          auth key, session state, RPC route
  → bff              one process, many gRPC services
  → biz / msg / …    existing chat services
```

Session does not switch on the TL method name itself. It switches on the generated gRPC service, using `BFFProxyClients.IDMap` in `teamgramd/etc/session.yaml` (the Docker copy is `teamgramd/etc2/session.yaml`):

```yaml
"/mtproto.RPCMessages": "bff.bff"
```

The key is a prefix of the generated method (`/mtproto.RPCMessages/messages_sendMessage`). The value is an etcd key. Today every routed prefix points at the single `bff.bff` process, which registers each service in `app/bff/bff/internal/server/server.go`. A prefix that is commented out never leaves session. `TryReturnFakeRpcResult` may answer a few of those with an empty object; every other unrouted method returns `ENTERPRISE_IS_BLOCKED`.

That split is the extension point. A new product surface is a new gRPC service with its own etcd key, its own MySQL schema, and its own Kafka topics. Chat tables stay the chat tables.

## Two ways to expose it

### 1. MTProto, behind session and bff

Use this when an official client, or a Tellomi build of one, should call the feature as a TL method.

1. Add constructors to the TL schema (`proto/schema/telegram_api_layer232.tl` or a successor layer) under a new namespace, for example `swipe` or `moments`. Run the existing codegen so `rpc_client_registers.go` gains `/mtproto.RPCSwipe/swipe_getCandidates`.
2. Implement a gRPC service that owns the feature. It is not a package inside `biz`. It has its own process, etcd key (`service.swipe`), and config file.
3. Add a thin bff handler that adapts the TL request to that gRPC API. Register it from `app/bff/bff/internal/server/server.go` only if it should live in the current bff binary. A large feature can be a second bff-style process; point its IDMap value at that process's etcd key instead of `bff.bff`.
4. Uncomment or add the IDMap line. Until that line is active, clients still hit the proxy fallback.

Custom constructors are ordinary TL. Clients that do not know them will not send them. A Tellomi client build, or a small companion, is what calls `swipe.*` and `moments.*`. Stock Telegram clients never will, and they do not need to.

### 2. Separate HTTP API

Use this when the caller is not an MTProto client: a web matcher, a moments web view, or a first-party app that already has a session and does not want a new TL layer.

`httpserver` (`teamgramd/etc/httpserver.yaml`, port 8801) is the current HTTP process. A new product should be its own HTTP process and its own port, not new routes bolted onto chat callbacks. It still uses the same etcd, and it resolves the caller through authsession rather than a second login.

Suggested ports, not bound by anything in the repo today:

| Service | HTTP | gRPC listen | etcd key |
| --- | --- | --- | --- |
| swipe | 8810 | 127.0.0.1:20810 | `service.swipe` |
| moments | 8811 | 127.0.0.1:20811 | `service.moments` |

## Auth

Session already turns an MTProto auth key into a user. The metadata passed into bff is `proto/mtproto/rpc/metadata`: `auth_id`, `perm_auth_key_id`, and `user_id`. A new TL handler reads `user_id` and passes it to the feature service. The feature service does not parse MTProto and does not look up auth keys itself.

An HTTP handler cannot see that metadata. The client sends the permanent auth key id it already has (or a short-lived token that bff minted from it). The HTTP service calls authsession, gets the same `user_id`, and rejects the request when the key is unknown or the user is logged out. There is no swipe-specific password and no second account table.

## Data

Each service owns its schema and its database. The precedent in this repo is `ChannelMySQL` and `StoryMySQL` on bff: a feature with its own tables, not extra columns on `users` or `messages`.

| Service | Database | What it stores |
| --- | --- | --- |
| swipe | `tellomi_swipe` | profiles used for matching, likes, passes, matches, blocks |
| moments | `tellomi_moments` | posts, media refs, comments, feeds |

User id is a foreign id, not a row the feature service inserts. Display name and avatar are read from `biz` when a response needs them. Match and post rows are never written into the message or dialog tables. If a match should show up as a chat, the feature service calls the existing chat API to open a conversation; it does not insert a dialog row itself.

Media bytes stay in dfs / MinIO. Moments stores the dfs file id and the photo or document id, the same way stories keep media ids in their own tables.

## Kafka

Existing topics are `Sync-T` (updates to sessions) and `Inbox-T` (message fanout). New features get new topics so a swipe consumer cannot stall message delivery.

| Topic | Producer | Consumer | Payload |
| --- | --- | --- | --- |
| `Swipe-T` | swipe service | swipe service, and bff if a match must become a chat update | like, match, unmatch |
| `Moments-T` | moments service | moments fanout worker | post created, deleted, commented |

Topic names are placeholders. Brokers stay the cluster already configured (`127.0.0.1:9092` in `teamgramd/etc`). A match that should wake the other user is published to `Sync-T` only at the edge, by bff, as a normal update. The swipe service does not publish session updates itself.

## Swipe matching

Own service. Candidates, likes, and matches are queries on `tellomi_swipe`. Ranking does not belong in `messages.getDialogs`.

MTProto shape, if a client needs it:

```
swipe.getCandidates#........ limit:int = swipe.Candidates;
swipe.decide#........ user_id:long like:Bool = swipe.Decision;
swipe.getMatches#........ offset_id:int limit:int = swipe.Matches;
```

The constructor ids are assigned when the schema is actually added. `swipe.Decision` carries `matched:Bool` and, when a match is new, the peer that bff has just ensured a chat with.

HTTP shape for the same operations: `GET /v1/candidates`, `POST /v1/decisions`, `GET /v1/matches`, authorized as above.

## Moments / feed

Own service. A moment is not a message and not a story row. Stories already have a path (`RPCStories` is registered in the bff process and is not in the session IDMap). Moments should not be stored as stories to reuse that path.

```
moments.send#........ media:InputMedia caption:string = Updates;
moments.getFeed#........ offset_id:int limit:int = moments.Feed;
moments.delete#........ id:int = Bool;
```

`moments.getFeed` returns posts plus the users bff resolved from `biz`. Pagination is the moments service's cursor, not a dialog pts.

HTTP: `POST /v1/posts`, `GET /v1/feed`, `DELETE /v1/posts/{id}`.

## Config placeholders

These blocks are not files in the tree. They show the keys a later change would add. Secrets stay in the environment, the same way SMS credentials do (`docs/sms-email.md`).

`teamgramd/etc/swipe.yaml` (not shipped):

```yaml
Name: service.swipe
ListenOn: 127.0.0.1:20810
Etcd:
  Hosts:
    - 127.0.0.1:2379
  Key: service.swipe
MySQL: root:@tcp(127.0.0.1:3306)/tellomi_swipe?charset=utf8mb4&parseTime=true
Redis:
  Host: 127.0.0.1:6379
Kafka:
  Brokers:
    - 127.0.0.1:9092
  Topic: Swipe-T
HTTP:
  Host: 0.0.0.0
  Port: 8810
AuthSession:
  Etcd:
    Hosts:
      - 127.0.0.1:2379
    Key: service.authsession
BizServiceClient:
  Etcd:
    Hosts:
      - 127.0.0.1:2379
    Key: service.biz_service
```

`teamgramd/etc/moments.yaml` (not shipped):

```yaml
Name: service.moments
ListenOn: 127.0.0.1:20811
Etcd:
  Hosts:
    - 127.0.0.1:2379
  Key: service.moments
MySQL: root:@tcp(127.0.0.1:3306)/tellomi_moments?charset=utf8mb4&parseTime=true
Redis:
  Host: 127.0.0.1:6379
Kafka:
  Brokers:
    - 127.0.0.1:9092
  Topic: Moments-T
HTTP:
  Host: 0.0.0.0
  Port: 8811
AuthSession:
  Etcd:
    Hosts:
      - 127.0.0.1:2379
    Key: service.authsession
BizServiceClient:
  Etcd:
    Hosts:
      - 127.0.0.1:2379
    Key: service.biz_service
DfsClient:
  Etcd:
    Hosts:
      - 127.0.0.1:2379
    Key: service.dfs
```

Session routing, only when the MTProto constructors exist (not shipped):

```yaml
# teamgramd/etc/session.yaml  BFFProxyClients.IDMap
"/mtproto.RPCSwipe": "bff.swipe"       # or bff.bff if the handler is in-process
"/mtproto.RPCMoments": "bff.moments"
```

Environment overrides, same pattern as `TELLOMI_SMS_PROVIDER`:

| Variable | Default | Purpose |
| --- | --- | --- |
| `TELLOMI_SWIPE_MYSQL` | empty | DSN for `tellomi_swipe`. Empty means the process does not start. |
| `TELLOMI_SWIPE_TOPIC` | `Swipe-T` | Kafka topic for match events. |
| `TELLOMI_MOMENTS_MYSQL` | empty | DSN for `tellomi_moments`. |
| `TELLOMI_MOMENTS_TOPIC` | `Moments-T` | Kafka topic for feed events. |
| `TELLOMI_MOMENTS_HTTP` | `:8811` | Moments HTTP listen address. |

No migration, binary, or yaml in `teamgramd/etc` is added for these services by this note.
