# Link preview API

Public read-only JSON for the Cloudflare Worker that renders tell.cc pages.
tell.cc is the t.me host. The API resolves the same path shapes Telegram's
t.me site does, and it returns only data that is already public.

## Endpoint

```
GET /linkpreview/v1/resolve?path=<t.me path>
```

Default listen address is **0.0.0.0:8801** (`TELLOMI_LINKPREVIEW_ADDR`, yaml `Listen`).

```bash
curl -sS -H "X-Tellomi-Linkpreview-Token: $TELLOMI_LINKPREVIEW_TOKEN" \
  'http://127.0.0.1:8801/linkpreview/v1/resolve?path=/newsroom'
```

`path` is a tell.cc / t.me path, with or without a leading slash, or a full
`https://tell.cc/...` or `https://t.me/...` URL. Query parameters that belong
to the link (`start`, `server`, `port`, `secret`, `user`, `pass`, `url`,
`text`) may be inside `path` or repeated as sibling query parameters.

`GET /linkpreview/v1/health` returns `{"ok":true,"schema_version":1}` and does
not require the token.

Do not run `httpserver` on 8801 at the same time. Its config uses the same port
and the binary is not started by `runall2.sh`.

## Environment

| Variable | Yaml field | Role |
|---|---|---|
| `TELLOMI_LINKPREVIEW_ADDR` | `Listen` | Listen address. Default `:8801`. |
| `TELLOMI_LINKPREVIEW_MYSQL_DSN` | `MysqlDSN` | Teamgram MySQL DSN. Falls back to `TEAMGRAM_MYSQL_DSN`. Required for usernames, invites, posts, stories, and packs. |
| `TELLOMI_LINKPREVIEW_TOKEN` | `Token` | Shared secret. When set, `/resolve` requires header `X-Tellomi-Linkpreview-Token` or `Authorization: Bearer <token>`. Empty disables the check. |
| `TELLOMI_S3_PUBLIC_MEDIA_BASE_URL` | `PublicMediaBaseURL` | Public origin for avatars and media thumbs. URLs are `{base}/{file_path}`. |
| `TELLOMI_LINKPREVIEW_PUBLIC_URL` | `PublicURL` | Origin used for signed media URLs when the S3 base is unset. |
| `TELLOMI_LINKPREVIEW_MEDIA_KEY` | `MediaKey` | HMAC key for signed media URLs. Defaults to a value derived from the token, or a per-process random key. |
| `TELLOMI_LINK_BASE_URL` | `LinkBaseURL` | Prefix for the `link` field. Default `https://tell.cc`. |
| `TELLOMI_LINKPREVIEW_RATE_PER_MIN` | `RatePerMin` | Sustained requests per client IP. Default `120`. |
| `TELLOMI_LINKPREVIEW_RATE_BURST` | `RateBurst` | Burst size. Default `40`. |
| `TELLOMI_LINKPREVIEW_CACHE_SECONDS` | `CacheSeconds` | `Cache-Control` max-age for hits, and the in-process TTL. Default `120`. Not-found responses use 30 seconds. |
| `TELLOMI_LINKPREVIEW_TRUST_PROXY` | `TrustProxy` | When `1` / `true`, rate-limit by `CF-Connecting-IP` or the first `X-Forwarded-For` hop. |
| `TELLOMI_MINIO_ENDPOINT` | `MinioEndpoint` | Optional MinIO endpoint for `GET /linkpreview/v1/media`. |
| `TELLOMI_MINIO_ACCESS_KEY` | `MinioAccessKey` | MinIO access key. |
| `TELLOMI_MINIO_SECRET_KEY` | `MinioSecretKey` | MinIO secret key. |
| `TELLOMI_MINIO_USE_SSL` | `MinioSSL` | `1` / `true` to use TLS. |
| `TELLOMI_MINIO_BUCKET_PHOTOS` | `MinioBucketPhotos` | Default `photos`. |
| `TELLOMI_MINIO_BUCKET_DOCUMENTS` | `MinioBucketDocuments` | Default `documents`. |
| `TELLOMI_MINIO_BUCKET_VIDEOS` | `MinioBucketVideos` | Default `videos`. |

Environment variables override `teamgramd/etc/linkpreview.yaml` (docker uses
`teamgramd/etc2/linkpreview.yaml`). The process is `bin/linkpreview -f ../etc/linkpreview.yaml`.

## Auth, rate limit, cache

- Missing or wrong token: **401** `unauthorized`. Production should set `TELLOMI_LINKPREVIEW_TOKEN` so only the Worker can call `/resolve`. The media route does not use this header. Browsers load images with the signature alone.
- Over the limit: **429** `rate_limited` and `Retry-After: 1`.
- Success: `Cache-Control: public, max-age=<CacheSeconds>` when no token is set, otherwise `private, max-age=<CacheSeconds>`.
- `not_found`: the same scope with `max-age` 30.
- `401`, `429`, and `500`: `Cache-Control: no-store`.
- Every JSON response sets `ETag`, `Vary: X-Tellomi-Linkpreview-Token`, `X-Content-Type-Options: nosniff`. A matching `If-None-Match` on a cached 200 returns **304**.

## What is public

Returned:

- Username targets in `username` (`active=1`, `deleted=0`) and `channels.username`: users, bots, channels, supergroups.
- User title (first + last name), about, and avatar only when that privacy key is empty or `privacyValueAllowAll`. The default is allow-all. Contacts-only, disallow-all, and any other rule hide that field. Restricted or deleted users are `not_found`.
- Public channel and supergroup posts (`/name/id`, `/s/name`, `/s/name/id`, `/c/id/msgid`). `/c/` is `not_found` when the channel has no username.
- Invite previews for a live `chat_invites` row (`/+hash`, `/joinchat/hash`): title, about, count, photo, `request_needed`. Revoked, expired, not-yet-started, or used-up invites are `not_found`. The invite is the authorization to see that preview, including a private chat.
- Stories at `/name/s/id` only when the story is public (`storyItem.public` or `privacyValueAllowAll`) and still active, or pinned.
- Sticker and emoji packs in `sticker_sets` (`public=1`, `deleted=0`).
- `/proxy`, `/socks`, `/share/url`, and `/boost` (plus `/boost/name` and `/name/boost` for a public channel or supergroup). These values come from the link itself.

Never returned: phone numbers, bot tokens, access hashes, secret keys, private chats without an invite, private channel posts, non-public stories, contact phone numbers inside a post (the excerpt keeps the display name only). A restricted message (`restriction_reason`) is `not_found`.

Usernames are 5–32 characters, start with a letter, and do not end with `_`.

## Media URLs

`avatar_url` and `media_url` are absolute URLs on `TELLOMI_S3_PUBLIC_MEDIA_BASE_URL` when that variable is set:

```
https://cdn.example/media/photos/0/123.jpg
```

Otherwise they are signed URLs on this service:

```
GET /linkpreview/v1/media?p=<file path>&exp=<unix>&sig=<hex hmac-sha256>
```

The signature is HMAC-SHA256 over `path || 0x00 || exp` with `TELLOMI_LINKPREVIEW_MEDIA_KEY`, or a key derived from the link-preview token. Expiry is 24 hours. A valid signature with MinIO configured streams the object from the photos, documents, or videos bucket. A valid signature with no backend returns **404** `media_unavailable`. A bad signature is **404** `not_found`.

The object key is `photo_sizes.file_path` (preview size `m`, then `x`, `y`, `b`, `a`, `c`, `s`, `w`) or, for an image document with no thumb, `documents.file_path`.

## Paths

| Path | `kind` |
|---|---|
| `/username` | `user`, `bot`, `channel`, or `group` |
| `/username?start=` `startgroup` `startchannel` `startattach` | `bot` (the parameter is kept only for bots) |
| `/+hash`, `/joinchat/hash` | `invite` |
| `/channel/msgid`, `/s/channel/msgid`, `/c/id/msgid` | `message` |
| `/s/channel` | `feed` (up to 10 newest visible posts) |
| `/username/s/storyid` | `story` |
| `/addstickers/short`, `/addemoji/short` | `sticker_set`, `emoji_set` |
| `/proxy?server&port&secret` | `proxy` |
| `/socks?server&port&user&pass` | `socks` |
| `/share/url?url&text` | `share` (`text` is capped at 500 characters; `url` must be http or https) |
| `/boost`, `/boost/username`, `/username/boost` | `boost` |

`/c/id` accepts the channel id or a `-100<id>` string. Message ids are positive.

## Status codes

| HTTP | `error` | When |
|---|---|---|
| 200 | | Resolved. |
| 304 | | `If-None-Match` matched a cached 200. |
| 400 | `invalid_path` | Empty, unknown shape, or bad proxy/share parameters. |
| 401 | `unauthorized` | Token missing or wrong. |
| 404 | `not_found` | Unknown or private entity. |
| 404 | `media_unavailable` | Signed media URL with no S3 base and no MinIO. |
| 429 | `rate_limited` | Per-IP limit. |
| 500 | `internal` | Database error. The body does not include the driver error. |

## JSON schema

`schema_version` is **1**. Omitted fields are absent, not null. `peer.verified`, `peer.scam`, `peer.fake`, and `peer.premium` are always present on a `peer` object. `posts` is present for `feed` and is `[]` when the channel has no previewable posts.

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://tell.cc/linkpreview/v1/schema.json",
  "oneOf": [{ "$ref": "#/$defs/success" }, { "$ref": "#/$defs/error" }],
  "$defs": {
    "error": {
      "type": "object",
      "additionalProperties": false,
      "required": ["ok", "schema_version", "error"],
      "properties": {
        "ok": { "const": false },
        "schema_version": { "const": 1 },
        "error": {
          "enum": ["not_found", "invalid_path", "unauthorized", "rate_limited", "internal", "media_unavailable"]
        },
        "path": { "type": "string" }
      }
    },
    "success": {
      "type": "object",
      "additionalProperties": false,
      "required": ["ok", "schema_version", "kind", "path", "link"],
      "properties": {
        "ok": { "const": true },
        "schema_version": { "const": 1 },
        "kind": {
          "enum": ["user", "bot", "channel", "group", "invite", "message", "feed", "sticker_set", "emoji_set", "proxy", "socks", "share", "boost", "story"]
        },
        "path": { "type": "string" },
        "link": { "type": "string" },
        "peer": { "$ref": "#/$defs/peer" },
        "invite": { "$ref": "#/$defs/invite" },
        "post": { "$ref": "#/$defs/post" },
        "posts": { "type": "array", "items": { "$ref": "#/$defs/post" } },
        "pack": { "$ref": "#/$defs/pack" },
        "proxy": { "$ref": "#/$defs/proxy" },
        "socks": { "$ref": "#/$defs/socks" },
        "share": { "$ref": "#/$defs/share" },
        "story": { "$ref": "#/$defs/story" }
      },
      "allOf": [
        { "if": { "properties": { "kind": { "enum": ["user", "bot", "channel", "group"] } } }, "then": { "required": ["peer"] } },
        { "if": { "properties": { "kind": { "const": "invite" } } }, "then": { "required": ["invite"] } },
        { "if": { "properties": { "kind": { "const": "message" } } }, "then": { "required": ["peer", "post"] } },
        { "if": { "properties": { "kind": { "const": "feed" } } }, "then": { "required": ["peer", "posts"] } },
        { "if": { "properties": { "kind": { "enum": ["sticker_set", "emoji_set"] } } }, "then": { "required": ["pack"] } },
        { "if": { "properties": { "kind": { "const": "proxy" } } }, "then": { "required": ["proxy"] } },
        { "if": { "properties": { "kind": { "const": "socks" } } }, "then": { "required": ["socks"] } },
        { "if": { "properties": { "kind": { "const": "share" } } }, "then": { "required": ["share"] } },
        { "if": { "properties": { "kind": { "const": "story" } } }, "then": { "required": ["peer", "story"] } }
      ]
    },
    "peer": {
      "type": "object",
      "additionalProperties": false,
      "required": ["kind", "verified", "scam", "fake", "premium"],
      "properties": {
        "kind": { "enum": ["user", "bot", "channel", "group"] },
        "title": { "type": "string" },
        "username": { "type": "string" },
        "about": { "type": "string", "description": "Omitted when the about privacy rule is not allow-all." },
        "avatar_url": { "type": "string", "description": "Omitted when the profile photo is private or missing." },
        "verified": { "type": "boolean" },
        "scam": { "type": "boolean" },
        "fake": { "type": "boolean" },
        "premium": { "type": "boolean" },
        "member_count": { "type": "integer", "description": "Channels and groups only. Subscriber or member count." },
        "broadcast": { "type": "boolean" },
        "megagroup": { "type": "boolean" },
        "start": {
          "type": "object",
          "additionalProperties": false,
          "required": ["param", "value"],
          "properties": {
            "param": { "enum": ["start", "startgroup", "startchannel", "startattach"] },
            "value": { "type": "string", "maxLength": 64 }
          }
        }
      }
    },
    "invite": {
      "type": "object",
      "additionalProperties": false,
      "required": ["member_count", "request_needed"],
      "properties": {
        "title": { "type": "string" },
        "about": { "type": "string" },
        "avatar_url": { "type": "string" },
        "member_count": { "type": "integer" },
        "request_needed": { "type": "boolean" },
        "broadcast": { "type": "boolean" },
        "megagroup": { "type": "boolean" }
      }
    },
    "post": {
      "type": "object",
      "additionalProperties": false,
      "required": ["message_id"],
      "properties": {
        "message_id": { "type": "integer" },
        "date": { "type": "integer", "description": "Unix seconds." },
        "excerpt": { "type": "string", "description": "Whitespace-collapsed text, at most 180 characters. Contact posts use the display name only." },
        "media_type": { "enum": ["photo", "video", "document", "voice", "webpage", "poll", "geo", "contact", "other"] },
        "media_url": { "type": "string" },
        "views": { "type": "integer" }
      }
    },
    "pack": {
      "type": "object",
      "additionalProperties": false,
      "required": ["short_name", "count", "emoji"],
      "properties": {
        "short_name": { "type": "string" },
        "title": { "type": "string" },
        "count": { "type": "integer" },
        "avatar_url": { "type": "string" },
        "emoji": { "type": "boolean" }
      }
    },
    "proxy": {
      "type": "object",
      "additionalProperties": false,
      "required": ["server", "port", "secret"],
      "properties": {
        "server": { "type": "string" },
        "port": { "type": "integer", "minimum": 1, "maximum": 65535 },
        "secret": { "type": "string", "description": "MTProto proxy secret copied from the share link." }
      }
    },
    "socks": {
      "type": "object",
      "additionalProperties": false,
      "required": ["server", "port"],
      "properties": {
        "server": { "type": "string" },
        "port": { "type": "integer", "minimum": 1, "maximum": 65535 },
        "user": { "type": "string" },
        "pass": { "type": "string", "description": "SOCKS password copied from the share link." }
      }
    },
    "share": {
      "type": "object",
      "additionalProperties": false,
      "required": ["url"],
      "properties": {
        "url": { "type": "string" },
        "text": { "type": "string" }
      }
    },
    "story": {
      "type": "object",
      "additionalProperties": false,
      "required": ["story_id", "pinned"],
      "properties": {
        "story_id": { "type": "integer" },
        "date": { "type": "integer" },
        "expire_date": { "type": "integer" },
        "caption": { "type": "string" },
        "media_type": { "enum": ["photo", "video", "document", "voice", "webpage", "poll", "geo", "contact", "other"] },
        "media_url": { "type": "string" },
        "pinned": { "type": "boolean" }
      }
    }
  }
}
```

### Examples

User:

```json
{
  "ok": true,
  "schema_version": 1,
  "kind": "user",
  "path": "/adalov",
  "link": "https://tell.cc/adalov",
  "peer": {
    "kind": "user",
    "title": "Ada Lovelace",
    "username": "adalov",
    "about": "analyst",
    "avatar_url": "https://cdn.example/media/photos/ada.jpg",
    "verified": true,
    "scam": false,
    "fake": false,
    "premium": false
  }
}
```

Bot deep link (`peer.start` is omitted when the query has no start parameter):

```json
{
  "ok": true,
  "schema_version": 1,
  "kind": "bot",
  "path": "/helperbot?start=ref_1",
  "link": "https://tell.cc/helperbot?start=ref_1",
  "peer": {
    "kind": "bot",
    "title": "Helper",
    "username": "helperbot",
    "about": "I help",
    "verified": false,
    "scam": false,
    "fake": false,
    "premium": false,
    "start": { "param": "start", "value": "ref_1" }
  }
}
```

Channel post:

```json
{
  "ok": true,
  "schema_version": 1,
  "kind": "message",
  "path": "/newsroom/15",
  "link": "https://tell.cc/newsroom/15",
  "peer": {
    "kind": "channel",
    "title": "News",
    "username": "newsroom",
    "about": "daily",
    "avatar_url": "https://cdn.example/media/photos/news-m.jpg",
    "verified": true,
    "scam": false,
    "fake": false,
    "premium": false,
    "member_count": 1200,
    "broadcast": true
  },
  "post": {
    "message_id": 15,
    "date": 1700000000,
    "excerpt": "hello channel",
    "media_type": "photo",
    "media_url": "https://cdn.example/media/photos/news-m.jpg",
    "views": 8
  }
}
```

Feed (`kind` `feed`) uses the same `peer` plus `posts`, newest first. Invite:

```json
{
  "ok": true,
  "schema_version": 1,
  "kind": "invite",
  "path": "/+Abcd1234efgh5678ijkl",
  "link": "https://tell.cc/+Abcd1234efgh5678ijkl",
  "invite": {
    "title": "Secret chat",
    "about": "come in",
    "avatar_url": "https://cdn.example/media/photos/inv.jpg",
    "member_count": 3,
    "request_needed": true
  }
}
```

Story, pack, proxy, socks, share, and boost follow the same envelope. `boost` with no username has no `peer`. A targeted boost has `peer` for that public channel or supergroup and `path` `/boost/<username>`.

Not found:

```json
{ "ok": false, "schema_version": 1, "error": "not_found", "path": "/notauser" }
```

## Data sources

| Data | Tables |
|---|---|
| Usernames | `username`, `users`, `bots` (description and verified only; `bots.token` is never selected), `user_privacies` |
| Channels and posts | `channels`, `channel_messages`, `channel_feature` (`AvailableMinID` hides older posts) |
| Basic groups and invites | `chats`, `chat_invites` |
| Stories | `stories` |
| Avatars and thumbs | `photos`, `photo_sizes`, `documents` |
| Packs | `sticker_sets` |

`sticker_sets` is created on startup when MySQL is configured (`teamgramd/deploy/sql/migrate-20261029-linkpreview.sql` does the same). This server has no sticker-pack writer yet; a public pack is a row with `short_name`, `title`, `count`, `emoji` (`0` stickers, `1` custom emoji), `thumb_path`, `public=1`.

Channel and story message bodies are protobuf (`mtproto.Message`, `mtproto.StoryItem`), the same bytes the channel and story services store.
