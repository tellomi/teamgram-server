# Client regression

The suite in `e2e/client` speaks MTProto the way Telegram iOS and Android do.
In-memory handler tests live in `e2e/inmem` and run with `go test ./...`.
[gotd](https://github.com/gotd/td) does the obfuscated padded-intermediate
transport and the DH exchange against `teamgramd/bin/server_pkcs1.key`.
Every RPC is `invokeWithLayer` + `initConnection` at layer 232, encoded with
this repo's schema. gotd's generated API layer is not used for those calls.

## What it covers

| Test | What a failing run means |
|---|---|
| `TestInitConnectionInvokeWithLayer` | iOS and Android `initConnection` cannot read `help.getConfig`. |
| `TestHelpGetConfig` | `help.getConfig` did not return a config. |
| `TestHelpGetAppConfig` | `help.getAppConfig` returned 500, timed out, or did not decode. |
| `TestHelpGetTermsOfServiceUpdate` | The default config returns `help.termsOfServiceUpdateEmpty`, and a second call stays empty. If an update is returned, accepting its id does not make the next call empty. |
| `TestUpdatesGetStateAndDifference` | `updates.getState` / `updates.getDifference` did not return a difference object. |
| `TestStartupRPCs` | One startup RPC returned 500, timed out, or did not decode. A 4xx is recorded and does not fail this test. Subtests are named `ios/<method>` and `android/<method>`. |
| `TestPrivateChatReadOutbox` | A's private message shows a read outbox before B calls `messages.readHistory`, or `read_outbox_max_id` never advances after B reads up to its own id for that message. |
| `TestBasicGroupReadOutbox` | Same for a basic group. |
| `TestSupergroupReadOutbox` | Same for a megagroup (`channels.readHistory`). |
| `TestBadEntitiesDoNotBreakGetDifference` | A message with a zero-length entity and a duplicated entity makes `updates.getDifference` or `messages.getHistory` fail for the sender or the recipient. |
| `TestSendReactionThumbsUp` | The recipient reacts 👍 to a private message and it does not show up as `MessageReactions` on that message for both sides. The two sides number the same message differently, so this also catches a reaction stored under the wrong side's id. |
| `TestAvailableReactionsDownloadable` | `messages.getAvailableReactions` is empty, or a static icon's `upload.getFile` does not return bytes within 10s. |
| `TestStickerSetUploadGetFile` | `messages.getStickerSet` (Twemoji, then AnimatedEmoji) has no document, or `upload.getFile` does not return bytes within 10s. |
| `TestUsernameThreeCharacters` | A 3-character username cannot be checked and set. |
| `TestLinksUseTellCC` | `help.getConfig` `me_url_prefix` is not a `tell.cc` URL. |
| `TestReconnectAfterGatewayRestart` | After the `teamgram` container restarts, the previous auth key cannot `updates.getDifference` the message that was sent while the client was offline. |
| `TestCloudPasswordReloginAndDisable` | After `auth.logOut` on the same permanent key, `auth.signIn` does not return `SESSION_PASSWORD_NEEDED`, `account.getPassword` has no SRP challenge, `auth.checkPassword` does not authorize the key, or disabling 2FA with `passwordKdfAlgoUnknown` and an empty hash leaves `has_password` set. |
| `TestTranscribeAudioIsNotPremiumOrStub` | `messages.transcribeAudio` on a text message returns `PREMIUM_ACCOUNT_REQUIRED` or `TRANSCRIPTION_FAILED`, or a zero message id does not return `MSG_ID_INVALID`. |

Add a `Test...` function in `e2e/client` when a client bug shows up again. Keep the
name specific (`TestPrivateChatReadOutbox`, not `TestMessages`).

`TestZZImplementedSmoke` calls every method marked implemented at P0 or P1 in
`docs/api-coverage.csv`. It fails on `*_NOT_IMPLEMENTED` and on a decode error.
A business error from the dummy params is allowed. Regenerate the list with
`python3 e2e/client/gen_smoke.py` when coverage changes. How to add a case is
in `docs/testing.md`.

`TestStartupRPCs` is the list official clients issue while the chat list still
says Updating: `help.getConfig`, `help.getAppConfig`, `help.getTermsOfServiceUpdate`,
`updates.getState`, `updates.getDifference`, `messages.getDialogs`,
`messages.getPinnedDialogs`, `contacts.getContacts`, `account.getNotifySettings`,
`messages.getAllStickers`, `messages.getEmojiStickerGroups`,
`messages.getEmojiStatusGroups`, `messages.getEmojiProfilePhotoGroups`,
`messages.getAvailableReactions`, `messages.getTopReactions`,
`messages.getRecentReactions`, `account.getDefaultEmojiStatuses`, `stories.getAllStories`,
`stories.getPeerMaxIDs`, and the rest of the no-argument calls in
`e2e/client/startup.go` (langpack, privacy, themes, effects, saved dialogs, phone config).

## Accounts

`e2e/client/config.json` is the test config. Numbers are Telegram's `99966XYYYY`
shape. The code is the digit after `99966` five times, which is what
`teamgramd/etc2/bff.yaml` does when `TestPhones` is omitted:

| Name | Number | Code |
|---|---|---|
| alice | 9996610001 | 11111 |
| bob | 9996610002 | 11111 |
| terms | 9996610003 | 11111 |
| username | 9996610004 | 11111 |
| reconnect_a | 9996610005 | 11111 |
| reconnect_b | 9996610006 | 11111 |
| password | 9996610008 | 11111 |

`9996612345` is not code `12345`. The fixed code `12345` is only for numbers
outside this range.

Overrides: `TELLOMI_E2E_ADDR`, `TELLOMI_E2E_API_ID`, `TELLOMI_E2E_API_HASH`,
`TELLOMI_E2E_RSA_KEY`, `TELLOMI_E2E_PHONE_<NAME>`, `TELLOMI_E2E_CODE_<NAME>`,
`TELLOMI_E2E_CONFIG`.

## Run locally

Start the stack (`docs/local-docker.md`), then:

```bash
cd e2e/client
TELLOMI_E2E=1 TELLOMI_E2E_ADDR=127.0.0.1:10443 \
  go test -count=1 -timeout 40m .
```

`TELLOMI_E2E_RESTART=1` includes `TestReconnectAfterGatewayRestart`, which
runs `docker compose restart teamgram`. Leave it unset when the target is
not a local compose stack.

Without `TELLOMI_E2E`, only the offline checks run (layer wrapping and the
server key fingerprint).

## CI

`.github/workflows/e2e.yml` runs on every pull request. It starts
`docker-compose-env.yaml` and `docker-compose.yaml` (MySQL, Redis, Kafka in
KRaft, etcd, object storage, and the Teamgram image built in the job), then
`go test` in `e2e/client` with `TELLOMI_E2E_RESTART=1`.

Docker Hub no longer serves `minio/minio` or `minio/mc` without credentials,
and `dl.min.io` returns 410. `e2e/docker-compose.ci.yaml` is the CI overlay:
the service is still named `minio`, it still reads `MINIO_ROOT_USER` /
`MINIO_ROOT_PASSWORD`, and it still listens on port 9000. The process is
RustFS, which implements that S3 API. The AWS CLI creates the buckets
`minio-mc` used to create. The Chengdu compose file is not changed.
Monitoring, coturn, and MediaMTX are not started in CI.

### Required status check

The workflow does not change branch protection by itself. In the repository
settings, open **Branches → Branch protection** (or **Rulesets**) for
`master` and add this status check so a red run blocks merge:

**Client regression / e2e**

That string is the workflow name plus the job name. If the checks list shows
the job id alone, select `e2e`. Do not mark the workflow required until the
first run has completed, or GitHub will not offer the check name.

## Smoke after deploy

`scripts/smoke-e2e.sh` runs the subset that does not restart the server.
Point it at production or staging with the test accounts above. Production
must keep the default `99966` range (do not set `TestPhones: []`).

```bash
TELLOMI_E2E_ADDR=203.0.113.10:10443 ./scripts/smoke-e2e.sh
```

Staging on another port is `docs/staging.md`.
