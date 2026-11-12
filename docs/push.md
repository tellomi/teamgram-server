# Push notifications

iOS clients (rebranded Telegram-iOS, layer 232) register a device token with `account.registerDevice`. When a new message is delivered, sync sends an APNs HTTP/2 alert to tokens whose owner is the recipient. Android FCM (token type 2) and Huawei Push Kit (token type 13) are sent only when their own flags are on. Both default off. FCM is often unreachable from Chengdu; that attempt is logged and dropped and does not delay APNs, Huawei, or Xiaomi.

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
  - ${TELLOMI_FCM_CREDENTIALS_FILE:-/dev/null}:/etc/tellomi/fcm/service-account.json:ro
```

Point `TELLOMI_APNS_KEY_FILE` at the real `.p8` on the host. Point `TELLOMI_FCM_CREDENTIALS_FILE` at the Firebase service-account JSON. The default `/dev/null` keeps `docker compose up` working when push is not configured. The FCM and Huawei variables are listed in the Android section.

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

The iOS notification extension connects as a supplementary session and does not load the saved language pack, so its `initConnection.lang_code` is `en`. That overwrites the app language on the same auth key. The phone language stays in `auths.system_lang_code`. A push therefore uses the app language when it names a pack other than English (`zh-hans`, `zh-hant`, `ja`, …), and otherwise the phone language. `zh`, `zh-CN` and `zh-Hans` are simplified. `zh-Hant`, `zh-TW` and `zh-HK` are traditional. A saved `classic-zh-cn` uses the `zh-hans` pack, and a saved `taiwan` uses `zh-hant`. Every other language in `data/langpack` uses its iOS `PUSH_*` string. A pack with no translation of that key falls back along `base_lang_code` to English. Thai and Greek now have their own `PUSH_*` strings (machine-translated where the official export had none).

The body is the text after `|` in the iOS string (`PUSH_MESSAGE_AUDIO` → `🎤 语音消息` for zh-hans). The sender or chat name is the title, not part of the body. The lock-screen word Tellomi is the iOS app name. `Erel → erel` is the notification extension naming the recipient account after the sender when more than one account is signed in. The server title is only the sender name.

`account.unregisterDevice` deletes that token for the current user and `other_uids`.

`auth.logOut` deletes the device rows for the permanent auth key, and for the temporary key when the device was stored on it. A logout with no permanent key removes every device row for that user, because that unbind drops every session. `account.resetAuthorization` and `auth.resetAuthorizations` delete the rows of the sessions they revoke and leave the caller's row. `account.deleteAccount` and an enforcement reset of every session delete every device row for that user. A failure to delete the row is logged and does not fail the RPC. The next push also skips a row whose `auth_users` binding is missing or `deleted = 1`, so a row that survived the delete is not sent.

## Logged-out rows already in the database

Rows registered before that delete existed are removed by hand. The command is safe to run again; the second run deletes nothing. It does not print tokens or secrets.

Back up `devices` first. The dump holds tokens and secrets. Do not commit it.

```bash
mysqldump --single-transaction --no-tablespaces teamgram devices \
  > "devices-before-push-cleanup-$(date -u +%Y%m%dT%H%M%SZ).sql"
```

Use the same MySQL credentials as `TEAMGRAM_MYSQL_DSN`. Keep the file until a logged-in phone still gets a message and a logged-out phone does not. Then:

```bash
go run ./cmd/push-device-cleanup -dry-run
go run ./cmd/push-device-cleanup -backup-done
```

`-dry-run` prints the count and does not write. Without `-backup-done` the command does not delete. `-dsn` overrides `TEAMGRAM_MYSQL_DSN` and is not logged.

This is not a startup migration. Deleting the rows needs the backup above, so it does not run when a process starts.

Telegram-iOS sends the 256-byte master notification key as `secret` when APNs encryption is on, and always for VoIP (`token_type` 9). The key id the extension looks up is the last 8 bytes of SHA1 of that secret. Message alerts skip VoIP tokens. An incoming call uses them.

## When a message is pushed

`sync.pushUpdates` is the recipient's new-message path. For each `updateNewMessage` that is not outgoing, sync loads that user's devices and sends. A device is included only when `auth_users` for that auth key and user is still live (`deleted = 0`). The rows are ordered newest first. Up to four devices are sent at the same time; each one is still logged on its own. Pushes are sent whether or not another session is online, which is what Telegram does: an iOS app in the background is often still marked online, and the client drops the banner while it is in the foreground.

The chat's `user_notify_settings` row wins over the global users/chats/channels row. `mute_until` in the future suppresses the push. A mention is still sent. `show_previews = 0` switches a text message to `MESSAGE_NOTEXT`, omits the message text, and leaves the outer `title` and `subtitle` off.

## APNs payload

HTTP/2 `POST /3/device/<hex token>` to `api.push.apple.com` (or the sandbox host), JWT `ES256` bearer from the `.p8`, `apns-push-type: alert`, `apns-priority: 10`, `apns-topic` set to the bundle id. Expiration is 24 hours so Apple stores the notification for an offline phone.

The outer JSON matches the Telegram loc-key form and carries the ids the app reads:

```json
{
  "aps": {
    "alert": {
      "title": "林",
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

`loc-key` / `loc-args` are the Telegram keys (`MESSAGE_TEXT`, `MESSAGE_PHOTO`, `MESSAGE_AUDIO`, `CHAT_MESSAGE_*`, `CHANNEL_MESSAGE_*`, `PINNED_*`, and the secret / gif / round / sticker / file / contact / geo / poll / quiz forms). `body` is the same phrase in the recipient's language, for clients that show a literal string. When `title` is set, `body` is the message text or the phrase without a `Name: ` prefix. Apple uses `loc-key` when the app has that string and ignores `body`. Official `MESSAGE_TEXT` is `%1$@: %2$@`, so that phone still shows the name inside the localized sentence. `title` is separate from that sentence.

A private chat sets `title` to the sender's display name. A group sets `title` to the group title and `subtitle` to the sender name. A channel sets `title` to the channel title. `show_previews = 0` omits `title` and `subtitle`. The name is a literal `title`. Official `MESSAGE_*` strings are body templates (`%1$@: %2$@` and the group and channel forms), so they stay on `loc-key` / `loc-args`. `NotificationService.swift` copies `title`, `subtitle`, and `body` from the decrypted alert. iOS shows this outer alert when the extension times out before that copy.

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

The JSON is the official `PHONE_CALL_REQUEST` shape that Telegram-iOS PushKit actually reads. `call_id`, `call_ah`, and `from_id` are strings on the envelope and inside the sealed object. The unpadded base64url `updates` blob appears once, inside `p`: Telegram-iOS decrypts `p` and parses that field, then reports CallKit only when the caller user is present. The envelope does not repeat `updates` or `custom.updates`; those extra copies made a production body 7001–7005 bytes and Apple rejected anything over 5120 before the device rang. A token with no secret is skipped, because a VoIP push the app cannot decrypt is not reported to CallKit.

The `updates` blob itself is packed the way official Telegram does when the body would otherwise be too large: only `updatePhoneCall` / `phoneCallRequested` and a slim caller `user` (id, access hash, name, phone, username, and identity flags; no photo, status, or color), then wrapped in `gzip_packed#3072cfa1` when that is smaller. Telegram-iOS `AccountStateManager.extractIncomingCallUpdate` unwraps that constructor first. A body that is still over 5120 bytes is not sent; sync logs `push: build voip ... voip payload is N bytes, limit 5120` at error level.

The same call is also sent to FCM token type 2 and Huawei token type 13 when those senders are installed. See Android below. An FCM or Huawei row with no secret is skipped. APNs being unconfigured does not suppress the Android ring.

## Android (FCM, Huawei, and Xiaomi)

Production sends APNs when the `.p8` is mounted. `sync` constructs `push.Service` from `TELLOMI_APNS_*`, then installs Xiaomi, FCM, and Huawei only when each flag is `1`. Any other value, including unset, leaves that channel stored and unsent. `show_previews = 0` and a passcode (`account.updateDeviceLocked`) clear the message text before any of these senders run, the same way Xiaomi's title and description are built.

| token_type | Client | What this server does |
| --- | --- | --- |
| `1` | APNs alert | Sent when the `.p8` is mounted |
| `2` | FCM | Sent only when `TELLOMI_FCM_PUSH=1` and the service-account file loads. Otherwise stored (`ErrFCMNotConfigured`) |
| `9` | APNs VoIP | Sent for `phoneCallRequested` |
| `13` | Huawei Push Kit | Sent only when `TELLOMI_HUAWEI_PUSH=1` and the app id and secret are set. `tellomi/Telegram` `TMessagesProj_AppHuawei` registers this type |
| `14` | Xiaomi Push | Sent only when `TELLOMI_XIAOMI_PUSH=1` and the AppSecret and package name are set. Not a Telegram API type |

OPPO, vivo, Honor, Meizu, and JPush are not sent. `XiaomiUtilities` in the Android client is an MIUI permission helper, not this channel. A phone receives Xiaomi pushes only after the Tellomi Android build registers a MiPush regid as `token_type` 14. Other brands do not get this channel unless that build also ships the Xiaomi Push SDK. Honor Push is a separate channel from Huawei; a Honor phone that does not register token type 13 still has no sender.

### Xiaomi

`POST https://api.xmpush.xiaomi.com/v3/message/regid` (override with `TELLOMI_XIAOMI_HOST`). Header `Authorization: key=<TELLOMI_XIAOMI_APP_SECRET>`. The body is form fields: `registration_id`, `restricted_package_name` (`TELLOMI_XIAOMI_PACKAGE`), `title`, `description`, `payload`, `pass_through=0`, `notify_type` (`-1`, or `0` when the chat is silent), `time_to_live=86400000`, `extra.notify_foreground=0`.

`pass_through=0` is a notification-bar message, so the banner can show when the process is not running. `payload` is the same `p` ciphertext Huawei would send when the device registered a secret. Without a secret, `payload` is the FCM data JSON (`loc_key`, string `loc_args`, `custom`, `body`). The client decrypts `p` the same way `GcmPushListenerService` does. Code `20301` deletes that device row. Any other MiPush error is logged and the row stays.

| Variable | Default | Purpose |
| --- | --- | --- |
| `TELLOMI_XIAOMI_PUSH` | off | `1` installs the sender. Any other value leaves token type 14 stored and unsent |
| `TELLOMI_XIAOMI_APP_SECRET` | empty | MiPush AppSecret. Not logged. Flag on with an empty secret logs once and does not send |
| `TELLOMI_XIAOMI_PACKAGE` | empty | `restricted_package_name`, the Android application id |
| `TELLOMI_XIAOMI_HOST` | `https://api.xmpush.xiaomi.com` | China endpoint. Overseas regids need the matching regional host |

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

`body` is the localized fallback. The official client ignores it and formats `loc_key` with `values-zh`. `show_previews = 0` switches the key to `MESSAGE_NOTEXT` and omits the message text from `body`, `loc_args`, and the sealed object inside `p`.

### FCM

`POST https://fcm.googleapis.com/v1/projects/<project>/messages:send` (override the origin with `TELLOMI_FCM_HOST`). Header `Authorization: Bearer <oauth access token>`. The access token is an OAuth JWT-bearer grant (`scope` `https://www.googleapis.com/auth/firebase.messaging`) signed with the service-account RSA key. The token endpoint is `token_uri` from that file, or `TELLOMI_FCM_TOKEN_URL`, or `https://oauth2.googleapis.com/token`.

The body is a data message. There is no `notification` block, so FCM cannot draw a banner from the message text. `message.data` is the string map above. `message.android.priority` is `HIGH`. `ttl` is `86400s`, or `60s` when `loc_key` is `PHONE_CALL_REQUEST`.

A call uses the same string map with `loc_key` `PHONE_CALL_REQUEST`, `call_id`, `call_ah`, `from_id`, `updates`, and `p`. The sealed object uses `loc_key` (underscore) plus those call fields. A row with no secret is skipped.

Each HTTP call gives up after 8 seconds. Token fetch plus send, including one 401 retry, gives up after 12 seconds. The attempt runs on its own goroutine and does not take a slot from the Deliver deadline. If 32 attempts are already in flight, the new one is logged and dropped. `UNREGISTERED`, `SENDER_ID_MISMATCH`, `NOT_FOUND`, and `INVALID_ARGUMENT` that names the registration token delete that device row. A timeout, an unreachable dial, or a payload error is logged and the row stays.

| Variable | Default | Purpose |
| --- | --- | --- |
| `TELLOMI_FCM_PUSH` | off | `1` installs the sender. Any other value leaves token type 2 stored and unsent |
| `TELLOMI_FCM_CREDENTIALS` | empty | Path of the Firebase service-account JSON. Not committed. The private key is not logged. Flag on with a missing or unusable file logs once and does not send |
| `TELLOMI_FCM_CREDENTIALS_FILE` | `/dev/null` | Host path mounted into the container at `/etc/tellomi/fcm/service-account.json`. Compose sets `TELLOMI_FCM_CREDENTIALS` to that container path when the variable is empty |
| `TELLOMI_FCM_PROJECT` | file `project_id` | Overrides the FCM project id in the URL |
| `TELLOMI_FCM_HOST` | `https://fcm.googleapis.com` | Send origin |
| `TELLOMI_FCM_TOKEN_URL` | file `token_uri`, else `https://oauth2.googleapis.com/token` | OAuth token endpoint |

Chengdu often cannot open either Google host. Leave the flag off there unless a path to Google exists. Huawei and Xiaomi do not wait on this dial.

### Huawei

`POST https://push-api.cloud.huawei.com/v1/<app id>/messages:send` (override the origin with `TELLOMI_HUAWEI_HOST`). Header `Authorization: Bearer <access token>`. The token is `client_credentials` at `https://oauth-login.cloud.huawei.com/oauth2/v3/token` (`TELLOMI_HUAWEI_TOKEN_URL` overrides it), with `client_id` = `TELLOMI_HUAWEI_APP_ID` and `client_secret` = `TELLOMI_HUAWEI_APP_SECRET`. The secret is not logged.

The body is a data message. `message.data` is the raw `p` string, not the FCM wrapper. `message.token` is that one registration token. `android.urgency` is `HIGH`, `android.ttl` is `86400s`, and `android.category` is `IM`. A call uses `SendCall`: `ttl` `60s` and `category` `VOIP`. There is no notification title, so a hidden preview cannot show up in the system tray. The listener passes `data` straight to the decryptor.

Each HTTP call gives up after 8 seconds, and that budget also covers the token fetch. Code `80000000` is success. Code `80300007` (every token invalid) and `80100000` (this token is in `illegal_tokens`) delete the device row. An auth failure refreshes the token once and does not delete the row. Any other error is logged and the row stays.

| Variable | Default | Purpose |
| --- | --- | --- |
| `TELLOMI_HUAWEI_PUSH` | off | `1` installs the sender. Any other value leaves token type 13 stored and unsent |
| `TELLOMI_HUAWEI_APP_ID` | empty | Push Kit client id. Flag on with an empty id logs once and does not send |
| `TELLOMI_HUAWEI_APP_SECRET` | empty | Push Kit client secret. Not logged |
| `TELLOMI_HUAWEI_HOST` | `https://push-api.cloud.huawei.com` | China send origin |
| `TELLOMI_HUAWEI_TOKEN_URL` | `https://oauth-login.cloud.huawei.com/oauth2/v3/token` | China OAuth endpoint |

## Clients

iOS `tellomi/Telegram-iOS`:

- `NotificationService.swift` copies the decrypted `aps.alert.title` and `body`. A `loc-key` on that decrypted object is treated as a service event and the banner is not shown. Do not put `loc-key` in `p`.
- `zh-Hans.lproj/Localizable.strings` and `zh-Hant.lproj/Localizable.strings` now have the `MESSAGE_*` keys. `en.lproj` still names them `PUSH_MESSAGE_*`, so an English phone that never runs the extension shows the server `body` (the key is missing). The extension is what a locked phone actually shows, and it shows the server body.
- `standaloneStateManager` passes a nil language, so every notification rewrites `auths.lang_code` to `en`. The server uses `system_lang_code` in that case. The extension also appends ` → account` when more than one account is signed in; a private chat should leave that off. Those two client edits are in `cursor/push-lang-private-title-817a` and could not be pushed from this run (the token cannot write `tellomi/Telegram-iOS`).

Android `tellomi/Telegram`:

- `PushListenerController` handles `PUSH_TYPE_FIREBASE` (2) and `PUSH_TYPE_HUAWEI` (13) only. Both decrypt `p` and format `loc_key` with the app's `LocaleController` (`NotificationMessageAudio` and the rest). The server `body` is ignored, so the same payload is Chinese when the app language is Chinese.
- FCM is sent when `TELLOMI_FCM_PUSH=1`. Huawei is sent when `TELLOMI_HUAWEI_PUSH=1`. Xiaomi is token type 14 and is sent only when `TELLOMI_XIAOMI_PUSH=1`. The client has to register the regid and read `payload` as `p`.
- There is no Honor, OPPO, vivo, or Meizu sender. A Honor phone that does not register a Huawei token has no push path.
