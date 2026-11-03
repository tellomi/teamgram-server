# Push notifications

iOS clients (rebranded Telegram-iOS, layer 232) register a device token with `account.registerDevice`. When a new message is delivered, sync sends an APNs HTTP/2 alert to tokens whose owner is the recipient. Android FCM is stored and left unwired: mainland China has no FCM.

Muted chats do not notify, except when the message mentions the recipient. `show_previews` hides the message text. `silent` (and a silent message) sends the alert with no sound.

## What erel needs to provide

1. An Apple Developer **APNs Auth Key** (`.p8`, Key ID, Team ID). One key works for sandbox and production. Do not commit it.
2. The iOS bundle id. That is `TELLOMI_APNS_TOPIC`.
3. The key mounted into the `teamgram` container, and the five variables below in the environment or a gitignored `.env`.
4. TestFlight and App Store builds must use the **production** APNs host (`TELLOMI_APNS_SANDBOX=false`). Xcode debug tokens are sandbox; those devices set `app_sandbox` and are routed to the sandbox host even when the flag is false.

Without the key, registration still stores the token and no request is sent to Apple.

## Environment variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `TELLOMI_APNS_KEY_ID` | empty | Key ID of the `.p8` (Apple's 10-character id) |
| `TELLOMI_APNS_TEAM_ID` | empty | Apple Team ID, the JWT `iss` |
| `TELLOMI_APNS_TOPIC` | empty | Bundle id, sent as `apns-topic` for message alerts |
| `TELLOMI_APNS_VOIP_TOPIC` | `<TELLOMI_APNS_TOPIC>.voip` | PushKit topic for `token_type` 9. Leave unset. The same `.p8` signs it. |
| `TELLOMI_APNS_KEY_PATH` | `/etc/tellomi/apns/AuthKey.p8` | Path inside the container |
| `TELLOMI_APNS_SANDBOX` | `false` | `true` forces every push to `api.sandbox.push.apple.com`. Leave `false` for TestFlight. A device that registered `app_sandbox=true` still uses the sandbox host when this is false. |

`docker-compose.yaml` passes these into the `teamgram` service. The key mount is:

```yaml
environment:
  TELLOMI_APNS_KEY_ID: ${TELLOMI_APNS_KEY_ID:-}
  TELLOMI_APNS_TEAM_ID: ${TELLOMI_APNS_TEAM_ID:-}
  TELLOMI_APNS_TOPIC: ${TELLOMI_APNS_TOPIC:-}
  TELLOMI_APNS_VOIP_TOPIC: ${TELLOMI_APNS_VOIP_TOPIC:-}
  TELLOMI_APNS_KEY_PATH: ${TELLOMI_APNS_KEY_PATH:-/etc/tellomi/apns/AuthKey.p8}
  TELLOMI_APNS_SANDBOX: ${TELLOMI_APNS_SANDBOX:-false}
volumes:
  - ${TELLOMI_APNS_KEY_FILE:-/dev/null}:/etc/tellomi/apns/AuthKey.p8:ro
```

Point `TELLOMI_APNS_KEY_FILE` at the real `.p8` on the host. The default `/dev/null` keeps `docker compose up` working when push is not configured.

Devices and notify settings live in the existing teamgram MySQL database (`devices`, `user_notify_settings`). The bff uses `ChannelMySQL`; sync uses its `Mysql.DSN`. `TEAMGRAM_MYSQL_DSN` overrides both.

## Registration

`account.registerDevice` stores, per auth key (`perm_auth_key_id`, falling back to the temp auth key id):

| Column | Source |
| --- | --- |
| `token_type` | `1` APNs, `2` FCM, `9` APNs VoIP, and the other types from the API |
| `token` | device token (iOS sends hex) |
| `app_sandbox` | `app_sandbox` |
| `secret` | raw `secret` bytes, stored hex-encoded |
| `no_muted` | `flags.0` |
| `other_uids` | other accounts on the device |
| `lang_code` | app language from the session (`initConnection.lang_code`, else `system_lang_code`) |

When a push is built, `auths.lang_code` for that auth key wins over the device snapshot. `initConnection` rewrites `auths.lang_code` on every connection, so a language switch applies on the next message without waiting for another `account.registerDevice`.

The iOS notification extension connects as a supplementary session and does not load the saved language pack, so its `initConnection.lang_code` is `en`. That overwrites the app language on the same auth key. The phone language stays in `auths.system_lang_code`. A push therefore uses the app language when it names a pack other than English (`zh-hans`, `zh-hant`, `ja`, …), and otherwise the phone language. `zh`, `zh-CN` and `zh-Hans` are simplified. `zh-Hant`, `zh-TW` and `zh-HK` are traditional. A saved `classic-zh-cn` uses the `zh-hans` pack, and a saved `taiwan` uses `zh-hant`. Every other language in `data/langpack` uses its iOS `PUSH_*` string. A pack with no translation of that key falls back along `base_lang_code` to English. Thai has no strings of its own, so a Thai banner is English.

The body is the text after `|` in the iOS string (`PUSH_MESSAGE_AUDIO` → `🎤 语音消息` for zh-hans). The sender or chat name is the title, not part of the body. The lock-screen word Tellomi is the iOS app name. `Erel → erel` is the notification extension naming the recipient account after the sender when more than one account is signed in. The server title is only the sender name.

`account.unregisterDevice` deletes that token for the current user and `other_uids`.

Telegram-iOS sends the 256-byte master notification key as `secret` when APNs encryption is on, and always for VoIP (`token_type` 9). The key id the extension looks up is the last 8 bytes of SHA1 of that secret. Message alerts skip VoIP tokens. An incoming call uses them.

## When a message is pushed

`sync.pushUpdates` is the recipient's new-message path. For each `updateNewMessage` that is not outgoing, sync loads that user's devices and sends. Pushes are sent whether or not another session is online, which is what Telegram does: an iOS app in the background is often still marked online, and the client drops the banner while it is in the foreground.

The chat's `user_notify_settings` row wins over the global users/chats/channels row. `mute_until` in the future suppresses the push. A mention is still sent. `show_previews = 0` switches a text message to `MESSAGE_NOTEXT` and omits the body.

## APNs payload

HTTP/2 `POST /3/device/<hex token>` to `api.push.apple.com` (or the sandbox host), JWT `ES256` bearer from the `.p8`, `apns-push-type: alert`, `apns-priority: 10`, `apns-topic` set to the bundle id. Expiration is 24 hours so Apple stores the notification for an offline phone.

The outer JSON matches the Telegram loc-key form and carries the ids the app reads:

```json
{
  "aps": {
    "alert": {
      "loc-key": "MESSAGE_AUDIO",
      "loc-args": ["林"],
      "body": "🎤 语音消息"
    },
    "mutable-content": 1,
    "sound": "default",
    "thread-id": "3"
  },
  "from_id": "3",
  "msg_id": "42",
  "custom": {"from_id": "3", "msg_id": "42"},
  "p": "<base64url>"
}
```

`loc-key` / `loc-args` are the Telegram keys (`MESSAGE_TEXT`, `MESSAGE_PHOTO`, `MESSAGE_AUDIO`, `CHAT_MESSAGE_*`, `CHANNEL_MESSAGE_*`, `PINNED_*`, and the secret / gif / round / sticker / file / contact / geo / poll / quiz forms). `body` is the same phrase in the recipient's language, for clients that show a literal string. Apple uses `loc-key` when the app has that string and ignores `body`.

Groups use `chat_id` (not `from_id`). Channels use `channel_id`. The ids are strings.

`p` is required by `Telegram/NotificationService/Sources/NotificationService.swift`. The extension base64url-decodes it, takes the first 8 bytes as the notification key id, and decrypts with MTProto 2.0 (`x = 8`) using the master key registered as `secret`. The plaintext is a little-endian length plus JSON. That JSON must **not** contain `loc-key`: if it does, the extension treats the push as a service event and does not render `aps.alert`. The decrypted object therefore looks like:

```json
{
  "msg_id": "42",
  "from_id": "3",
  "aps": {
    "alert": {"title": "林", "body": "🎤 语音消息"},
    "mutable-content": 1,
    "thread-id": "3",
    "category": "m",
    "sound": "default"
  }
}
```

The extension copies those string ids into the notification and shows `title` / `body` as-is. It does not look up `MESSAGE_*` in the string catalog. A Chinese recipient therefore needs the server to put Chinese in `body`. Text messages still use the message text as `body`.

A pinned service message (`messageActionPinMessage`) uses `PINNED_NOTEXT` or `PINNED_TEXT`. Those keys are not prefixed with `CHAT_` or `CHANNEL_`; the peer is in `loc-args`. Reaction updates are not pushed: `updateMessageReactions` does not name a single actor, so a `REACT_*` payload would be a guess. The phrase table still has `REACT_*` / `CHAT_REACT_*` bodies for a later sender.

## Incoming calls (PushKit)

`sync.pushUpdates` of `updatePhoneCall` / `phoneCallRequested` also sends each `token_type` 9 row. A locked iOS app is often still an online MTProto session, so the VoIP push is sent as well as the online `updatePhoneCall`. The client dedupes CallKit by call id. `phoneCallWaiting` (the caller) is not a VoIP push. A request whose `updates` have no user for `admin_id` is not sent: PushKit would wake the app and the app would not be able to report the real call.

The request uses the same `.p8`, key id, team id, and sandbox flag as message alerts. The topic is `TELLOMI_APNS_VOIP_TOPIC`, or `<TELLOMI_APNS_TOPIC>.voip` when that is empty. On Chengdu that is `app.tellomi.chat` → `app.tellomi.chat.voip`. There is no second key and no `.p12`.

Headers: `apns-push-type: voip`, `apns-priority: 10`, `apns-expiration: 0`. Expiration stays 0 so APNs does not store the push.

The JSON is the official `PHONE_CALL_REQUEST` shape. `call_id`, `call_ah`, and `from_id` are strings. `custom.updates` and top-level `updates` are the unpadded base64url of a layer-232 `updates#74ae4240` containing `updatePhoneCall` / `phoneCallRequested` and the caller in `users`. `p` is that same JSON sealed with the device secret, the same MTProto 2.0 seal as a message push. Telegram-iOS decrypts `p`, parses `updates`, and reports CallKit only when the caller user is present. A token with no secret is skipped, because a VoIP push the app cannot decrypt is not reported to CallKit.

## Android (FCM and Huawei)

Production sends APNs only. `sync` constructs `push.Service` from `TELLOMI_APNS_*` and never calls `SetFCM` or `SetHuawei`.

| token_type | Client | What this server does |
| --- | --- | --- |
| `1` | APNs alert | Sent when the `.p8` is mounted |
| `2` | FCM | Stored. No sender is installed (`ErrFCMNotConfigured`). Mainland China usually cannot reach FCM |
| `9` | APNs VoIP | Sent for `phoneCallRequested` |
| `13` | Huawei Push Kit | Stored. No sender is installed (`ErrHuaweiNotConfigured`). `tellomi/Telegram` `TMessagesProj_AppHuawei` registers this type |

There is no Xiaomi, OPPO, vivo, Honor, Meizu, or JPush token in the Telegram API, and no code here sends one. `XiaomiUtilities` in the Android client is an MIUI permission helper, not a push transport. A non-Huawei phone in mainland China that cannot reach FCM has no push path.

The FCM data object (every value a string, which the FCM HTTP API requires) is:

```json
{
  "loc_key": "MESSAGE_PHOTO",
  "loc_args": "[\"林\"]",
  "custom": "{\"from_id\":\"3\",\"msg_id\":\"42\"}",
  "user_id": "9",
  "body": "📷图片",
  "p": "<base64url>"
}
```

`tellomi/Telegram` `GcmPushListenerService` reads only `p`. After decryption the JSON is the spec object, with a real array and object:

```json
{
  "loc_key": "MESSAGE_PHOTO",
  "loc_args": ["林"],
  "custom": {"from_id": "3", "msg_id": "42"},
  "user_id": 9,
  "body": "📷图片"
}
```

Huawei's listener passes the whole data string to that decryptor, so a future HMS sender must post `p` itself, not this wrapper. `body` is the localized fallback. The official client ignores it and formats `loc_key` with `values-zh`.

## Clients

iOS `tellomi/Telegram-iOS`:

- `NotificationService.swift` copies the decrypted `aps.alert.title` and `body`. A `loc-key` on that decrypted object is treated as a service event and the banner is not shown. Do not put `loc-key` in `p`.
- `zh-Hans.lproj/Localizable.strings` and `zh-Hant.lproj/Localizable.strings` now have the `MESSAGE_*` keys. `en.lproj` still names them `PUSH_MESSAGE_*`, so an English phone that never runs the extension shows the server `body` (the key is missing). The extension is what a locked phone actually shows, and it shows the server body.
- `standaloneStateManager` passes a nil language, so every notification rewrites `auths.lang_code` to `en`. The server uses `system_lang_code` in that case. The extension also appends ` → account` when more than one account is signed in; a private chat should leave that off. Those two client edits are in `cursor/push-lang-private-title-817a` and could not be pushed from this run (the token cannot write `tellomi/Telegram-iOS`).

Android `tellomi/Telegram`:

- `PushListenerController` handles `PUSH_TYPE_FIREBASE` (2) and `PUSH_TYPE_HUAWEI` (13) only. Both decrypt `p` and format `loc_key` with the app's `LocaleController` (`NotificationMessageAudio` and the rest). The server `body` is ignored, so the same payload is Chinese when the app language is Chinese.
- There is no Honor, Xiaomi, OPPO, vivo, or Meizu sender. A Honor phone without Google Play has no FCM path. The Huawei flavor registers token type 13, and this server still does not send it. That phone gets no banner, rather than an English one.
