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
| `TELLOMI_APNS_TOPIC` | empty | Bundle id, sent as `apns-topic` |
| `TELLOMI_APNS_KEY_PATH` | `/etc/tellomi/apns/AuthKey.p8` | Path inside the container |
| `TELLOMI_APNS_SANDBOX` | `false` | `true` forces every push to `api.sandbox.push.apple.com`. Leave `false` for TestFlight. A device that registered `app_sandbox=true` still uses the sandbox host when this is false. |

`docker-compose.yaml` passes these into the `teamgram` service. The key mount is:

```yaml
environment:
  TELLOMI_APNS_KEY_ID: ${TELLOMI_APNS_KEY_ID:-}
  TELLOMI_APNS_TEAM_ID: ${TELLOMI_APNS_TEAM_ID:-}
  TELLOMI_APNS_TOPIC: ${TELLOMI_APNS_TOPIC:-}
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

`account.unregisterDevice` deletes that token for the current user and `other_uids`.

Telegram-iOS sends the 256-byte master notification key as `secret` when APNs encryption is on, and always for VoIP (`token_type` 9). The key id the extension looks up is the last 8 bytes of SHA1 of that secret. VoIP tokens are stored for calls and are not used for message alerts.

## When a message is pushed

`sync.pushUpdates` is the recipient's new-message path. For each `updateNewMessage` that is not outgoing, sync loads that user's devices and sends. Pushes are sent whether or not another session is online, which is what Telegram does: an iOS app in the background is often still marked online, and the client drops the banner while it is in the foreground.

The chat's `user_notify_settings` row wins over the global users/chats/channels row. `mute_until` in the future suppresses the push. A mention is still sent. `show_previews = 0` switches a text message to `MESSAGE_NOTEXT` and omits the body.

## APNs payload

HTTP/2 `POST /3/device/<hex token>` to `api.push.apple.com` (or the sandbox host), JWT `ES256` bearer from the `.p8`, `apns-push-type: alert`, `apns-priority: 10`, `apns-topic` set to the bundle id. Expiration is 24 hours so Apple stores the notification for an offline phone.

The outer JSON matches the Telegram loc-key form and carries the ids the app reads:

```json
{
  "aps": {
    "alert": {"loc-key": "MESSAGE_TEXT", "loc-args": ["Ada Lovelace", "hello"]},
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

Groups use `chat_id` (not `from_id`). Channels use `channel_id`. The ids are strings.

`p` is required by `Telegram/NotificationService/Sources/NotificationService.swift`. The extension base64url-decodes it, takes the first 8 bytes as the notification key id, and decrypts with MTProto 2.0 (`x = 8`) using the master key registered as `secret`. The plaintext is a little-endian length plus JSON. That JSON must **not** contain `loc-key`: if it does, the extension treats the push as a service event and does not render `aps.alert`. The decrypted object therefore looks like:

```json
{
  "msg_id": "42",
  "from_id": "3",
  "aps": {
    "alert": {"title": "Ada Lovelace", "body": "hello"},
    "mutable-content": 1,
    "thread-id": "3",
    "category": "m",
    "sound": "default"
  }
}
```

The extension copies those string ids into the notification and shows the alert, then polls for the message.

## FCM

Token type `2` is saved. `push.Service.SetFCM` accepts a sender; the default returns `ErrFCMNotConfigured` and does not call out. The bytes it would send are the same JSON as APNs, including `p` when the device registered a secret. Fill that in when an Android transport exists.
