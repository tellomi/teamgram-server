# Client regression

The suite in `e2e/client` speaks MTProto the way Telegram iOS and Android do.
In-memory handler tests live in `e2e/inmem` and run with `go test ./...`.
[gotd](https://github.com/gotd/td) does the obfuscated padded-intermediate
transport and the DH exchange against `teamgramd/bin/server_pkcs1.key`
(upstream fingerprint `12240908862933197005`). Production also advertises the
rotated key; see [SECURITY-OPS.md](SECURITY-OPS.md).
Every RPC is `invokeWithLayer` + `initConnection` at layer 232, encoded with
this repo's schema. gotd's generated API layer is not used for those calls.

## What it covers

| Test | What a failing run means |
|---|---|
| `TestInitConnectionInvokeWithLayer` | iOS and Android `initConnection` cannot read `help.getConfig`. |
| `TestHelpGetConfig` | `help.getConfig` did not return a config. When `TELLOMI_E2E_DC_OPTIONS` is set, `dc_options` did not match that spec, `webfile_dc_id` was not 1, or `dc_txt_domain_name` was not `apv2.stel.com`. Unset (production smoke) does not check the list. |
| `TestLangPackBeforeLogin` | `langpack.getLanguages` offered `hi`, `th`, or `el`, a client that already selected Hindi could not load that pack, or `help.getConfig` `suggested_lang_code` did not follow `initConnection.system_lang_code` (a Hindi device with the app in Japanese must be suggested `ja`, not `hi`). |
| `TestHelpGetAppConfig` | `help.getAppConfig` returned 500, timed out, or did not decode. |
| `TestHelpGetTermsOfServiceUpdate` | The default config returns `help.termsOfServiceUpdateEmpty`, and a second call stays empty. If an update is returned, accepting its id does not make the next call empty. |
| `TestUpdatesGetStateAndDifference` | `updates.getState` / `updates.getDifference` did not return a difference object. |
| `TestStartupRPCs` | One startup RPC returned 500, timed out, or did not decode. A 4xx is recorded and does not fail this test. Subtests are named `ios/<method>` and `android/<method>`. |
| `TestPrivateChatReadOutbox` | A's private message shows a read outbox before B calls `messages.readHistory`, or `read_outbox_max_id` never advances after B reads up to its own id for that message. |
| `TestBasicGroupReadOutbox` | Same for a basic group. |
| `TestSupergroupReadOutbox` | Same for a megagroup (`channels.readHistory`). |
| `TestHistoryLimit100` | `messages.getHistory` with `limit=100` returns at most 50 messages from a channel that has 60, `offset_id` / `add_offset` / `min_id` / `max_id` select the wrong window, or `channels.getMessages` drops ids from that page. |
| `TestLangPackWarmIsFast` | The second `langpack.getLangPack` for android zh-hans is as slow as the first, a client already at the current version gets a non-empty difference, or `ChooseYourLanguage` is the same in zh-hans and en. |
| `TestFloodWaitOnImportContacts` | Seventy `updates.getState` calls return `FLOOD_WAIT`, a loop of `contacts.importContacts` never does, or a `getState` after that flood fails. |
| `TestBadEntitiesDoNotBreakGetDifference` | A message with a zero-length entity and a duplicated entity makes `updates.getDifference` or `messages.getHistory` fail for the sender or the recipient. |
| `TestSendReactionThumbsUp` | The recipient reacts 👍 to a private message and it does not show up as `MessageReactions` on that message for both sides. The two sides number the same message differently, so this also catches a reaction stored under the wrong side's id. |
| `TestAvailableReactionsDownloadable` | `messages.getAvailableReactions` is empty, or a static icon's `upload.getFile` does not return bytes within 10s. |
| `TestStickerSetUploadGetFile` | `messages.getStickerSet` (Twemoji, then AnimatedEmoji) has no document, or `upload.getFile` does not return bytes within 10s. |
| `TestCreateStickerPack` | iOS `messages.uploadMedia` of a WebP plus `stickers.createStickerSet` with an empty `short_name` does not return a sticker set whose document `upload.getFile` matches the upload, a non-sticker document is accepted into a pack, an unknown document does not return `STICKER_DOCUMENT_INVALID`, a missing upload part does not return `FILE_PART_1_MISSING`, or another user can delete the pack. |
| `TestUsernameFourCharacters` | A 4-character username cannot be checked and set, or a 3-character name is accepted. |
| `TestTellomiUsernameOccupied` | `account.checkUsername` for `tellomi`, `Tellomi`, or `TELLOMI` does not return `USERNAME_OCCUPIED`. |
| `TestOfficialUsernamesOccupied` | `account.checkUsername` for an appendix D name (`support`, `news`, `help`, `admin`, `stickers`, `premium`, or a case variant of `tellomi`) does not return `USERNAME_OCCUPIED`. |
| `TestProfileAndChannelNames` | With `TELLOMI_USERNAME_POLICY=1`, `account.updateProfile` does not return 400 `FIRSTNAME_INVALID` / `LASTNAME_INVALID` for `admin` / `tellomi`, a normal name or bio is not saved, a megagroup titled `admin` or described as `tellomi` is created, a basic group titled `admin` or `tellomi` or described as `admin` is saved, a normal Chinese basic-group title is rejected, or a chat message containing those words is rejected. |
| `TestLinksUseTellCC` | `help.getConfig` `me_url_prefix` is not a `tell.cc` URL. |
| `TestReconnectAfterGatewayRestart` | After the `teamgram` container restarts, the previous auth key cannot `updates.getDifference` the message that was sent while the client was offline. |
| `TestFreshLoginCannotResetOlderSession` | A session minutes old does not get `406 FRESH_RESET_AUTHORISATION_FORBIDDEN` from `auth.resetAuthorizations` and `account.resetAuthorization`, or the other device leaves the list or stops answering. |
| `TestLoginNoticeFromServiceUser` | A finished login does not leave a 777000 message naming that `initConnection` device and a Beijing minute, the other session cannot read the same message, or a rejected code adds another `新设备登录提醒`. |
| `TestQRLoginNoticeWithoutPassword` | A QR login with no cloud password does not leave a 777000 message naming the new device, the message names the scanner, or replaying the token or sending a bad token adds another notice. |
| `TestQRLoginNoticeWithPassword` | A QR scan on an account with a cloud password writes the 777000 message before `auth.checkPassword`, the message is missing after the proof, it names the scanner, or a second copy is written. |
| `TestLocalAgedResetDropsOtherSessions` | With `TELLOMI_E2E_LOCAL_RESET=1` and a loopback address, a synthetic user's local `auth_users.date_created` is moved back 25 hours and an aged session still cannot reset, a single reset or reset-all does not log the other key out on the next MTProto ping and request and on reconnect, a fresh third session can reset the aged one, or the device list disagrees. The test refuses a non-loopback address. gotd does not export `ping_delay_disconnect`; that constructor is covered by `TestNormalSessionDropsPingAfterRevocation`. |
| `TestCloudPasswordReloginAndDisable` | After `auth.logOut` on the same permanent key, `auth.signIn` does not return `SESSION_PASSWORD_NEEDED`, `account.getPassword` has no SRP challenge, `auth.checkPassword` does not authorize the key, or disabling 2FA with `passwordKdfAlgoUnknown` and an empty hash leaves `has_password` set. |
| `TestTranscribeAudioIsNotPremiumOrStub` | `messages.transcribeAudio` on a text message returns `PREMIUM_ACCOUNT_REQUIRED` or `TRANSCRIPTION_FAILED`, or a zero message id does not return `MSG_ID_INVALID`. |
| `TestInviteLinkSheet` | A supergroup or basic group invite sheet does not return a `https://tell.cc/+` link, `channels.getFullChannel` / `messages.getFullChat` omits that primary link, or creating an additional link with a title, `request_needed`, `expire_date`, and `usage_limit` fails. |
| `TestAnnounceFlagDefaultsOff` | `help.getAppConfig` is missing `tellomi_announce_enabled`, or the flag is true. The launch notice stays off unless `TELLOMI_ANNOUNCE=1` and a key are set. |
| `TestAboutDisclosureDefaultsOff` | `help.getAppConfig` is missing `tellomi_personalized_recommend_default`, that bool is true, or a filing number, model name, or registration number is published. Those stay off unless `TELLOMI_ABOUT_DISCLOSURE=1`. |
| `TestPremiumNotRequiredToContact` | `users.getIsPremiumRequiredToContact` returns an error or does not return one bool per requested user. |
| `TestRichTranslateAndCompose` | `messages.translateRichMessage` or `messages.composeRichMessageWithAI` returns `ENTERPRISE_IS_BLOCKED`, a valid translate does not return `TRANSLATE_REQ_FAILED` while `TELLOMI_TRANSLATE` is off, or proofread / compose-translate does not return `AICOMPOSE_TIMEOUT` while `TELLOMI_AI_COMPOSE` is off. |
| `TestConfirmBotConnection` | `account.confirmBotConnection` returns `ENTERPRISE_IS_BLOCKED`, a person or an empty id is not `BOT_ID_INVALID`, or a bot from `bots.createBot` does not confirm. |
| `TestMediaDedupTwoAccounts` | Two accounts uploading the same video do not share one `documents.file_path` and one `media_blobs` row, either download fails, deleting one message breaks the other download, `messages.getDocumentByHash` misses that sha256+size+mime, or a different mime does not return `documentEmpty`. |
| `TestMediaDedupPhotoGifAndFile` | Two accounts uploading the same JPEG as a photo, the same JPEG as an image document, the same plain file, or the same small GIF do not share one blob and one object key, either download fails, deleting one photo message breaks the other photo, a different JPEG reuses the first photo's object, or a document thumb size is recorded for only one of the two uploads. |
| `TestMediaFileHashesAndBlobGC` | `upload.getFileHashes` does not return the 128 KiB SHA-256 parts of the file, `offset` does not skip earlier parts, a negative offset is not `OFFSET_INVALID`, deleting one message releases the blob, retention deletes the object while another `media_blob_refs` row remains, or the last reference leaves a `media_blobs` row or a `media_blob_gc` row. |
| `TestAccountEnforcementBanAndLift` | The isolated phone or email account did not follow the sanction chain: 777000 notice delivery, a temporary freeze that still logs in and rejects `messages.sendMessage`, a permanent sign-in without a session, disposition notice and appeal, fake-model lift of only that case, or the human revoke audit. Skipped unless `TELLOMI_ENFORCEMENT_E2E=1`. |
| `TestAccountEnforcementReportConfirm` | A user report did not reach @erel from @tellomi, 「是」 did not ban with a 777000 notice and a disposition appeal, 「否」 banned someone, or a non-admin 「是」 banned someone. Skipped unless `TELLOMI_ENFORCEMENT_E2E=1`. The model is fakebailian. |

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
| notice | 9996610071 | 11111 |
| aged | 9996610072 | 11111 |
| qr | 9996610073 | 11111 |
| qrpass | 9996610074 | 11111 |
| folders | 9996610091 | 11111 |

`9996612345` is not code `12345`. The fixed code `12345` is only for numbers
outside this range.

Overrides: `TELLOMI_E2E_ADDR`, `TELLOMI_E2E_API_ID`, `TELLOMI_E2E_API_HASH`,
`TELLOMI_E2E_RSA_KEY`, `TELLOMI_E2E_PHONE_<NAME>`, `TELLOMI_E2E_CODE_<NAME>`,
`TELLOMI_E2E_CONFIG`.

## Run locally

Start the stack (`docs/local-docker.md`). An Android emulator on that same machine uses the existing `local` profile: `10.0.2.2` is the emulator's name for the host loopback, so `10.0.2.2:10443` is this stack's `127.0.0.1:10443`. An emulator on another machine is that machine's loopback. Then:

```bash
cd e2e/client
TELLOMI_E2E=1 TELLOMI_E2E_ADDR=127.0.0.1:10443 \
  go test -count=1 -timeout 40m .
```

`TELLOMI_E2E_RESTART=1` includes `TestReconnectAfterGatewayRestart`, which
runs `docker compose restart teamgram`. Leave it unset when the target is
not a local compose stack.

`TELLOMI_E2E_LOCAL_RESET=1` includes `TestLocalAgedResetDropsOtherSessions`.
It sets `auth_users.date_created` back 25 hours for the synthetic `aged`
user inside the local `mysql` container. The address must be loopback. Leave
it unset for any smoke pointed outside this machine.

Without `TELLOMI_E2E`, only the offline checks run (layer wrapping and the
server key fingerprint).

`TestProfileAndChannelNames` needs `TELLOMI_USERNAME_POLICY=1` on the server.
The CI compose overlay sets it. A local stack does not, unless `.env` does.

`TestAccountEnforcementBanAndLift` needs the CI overlay, not the default
compose file. Before teamgram starts, `go run ./e2e/enforcementmigrate`
creates database `tellomi_enforcement_e2e` on local MySQL. The overlay sets
`TELLOMI_ENFORCEMENT_STORE=mysql`, `TELLOMI_ENFORCEMENT_CREATE=1`,
`TELLOMI_ENFORCEMENT_REVIEW=1`, `TELLOMI_ENFORCEMENT_ADMINS=erel`, and points
`TELLOMI_AI_BASE_URL` at the
`fakebailian` container. `TestAccountEnforcementReportConfirm` uses that same stack. That process is `e2e/fakebailian/server.py`. It is
not DashScope and it holds no production key. The disposition and operator
routes listen on `127.0.0.1:8819` inside the teamgram container only. The
test reaches them with `docker compose exec`. A local run also needs
`TELLOMI_E2E=1` and `TELLOMI_ENFORCEMENT_E2E=1`. Without those, the test
skips. Production compose and `.env` do not set them.

## CI

`.github/workflows/e2e.yml` runs on every pull request. The job sets
`TELLOMI_DC_OPTIONS=1:127.0.0.1:10443,1:127.0.0.1:5222` for the teamgram
container and the same value as `TELLOMI_E2E_DC_OPTIONS` for
`TestHelpGetConfig`. `TELLOMI_PUBLIC_DC_HOST` is empty, so that address is
not rewritten. It is the CI host, not production.
The job starts
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

## Loopback and published ports

The client address in this file is `127.0.0.1:10443`. That is where the e2e
process dials. `docker-compose.yaml` publishes teamgram as `10443:10443`
with no host IP, so Docker binds that port on `0.0.0.0`. A client that uses
loopback does not by itself stop another interface from reaching the same
port.

`e2e/docker-compose.loopback.yaml` is a test-only overlay. It replaces the
teamgram host binds with `127.0.0.1`. It does not change container listeners,
production `.env`, or the default compose file, and it is not applied unless
that `-f` is passed. Check the merge without starting anything:

```bash
docker compose -f docker-compose-env.yaml -f docker-compose.yaml -f e2e/docker-compose.loopback.yaml config
```
