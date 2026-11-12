# Upstream patches

Tellomi changes on top of teamgram. Keep these small and listed here.

## Authorization hash, revocation and TTL

`auth_users.hash` used to be replaced on every `BindAuthKeyUser`. A second `auth.signIn` on the same permanent key (retry, future-auth token, cloud-password step) made `account.resetAuthorization` return `HASH_INVALID` for the hash the other device had just listed. A live row now keeps its hash. A row that was logged out, reset, or expired takes a new non-zero hash, and the 24 hour gate still uses `date_created`.

`auth.bindTempAuthKey` used to copy the user onto the permanent key and leave the temporary key's `auth_users` row in place, so one login was two authorizations. The temporary row is soft-deleted after the permanent row exists.

A revoked session answered the next protected RPC with `401 AUTH_KEY_UNREGISTERED` and dropped `ping` / `ping_delay_disconnect` with no error, so the long connection stayed up and the client did not return to the login page. Both now receive that 401. `account.setAuthorizationTTL` used to store `users.authorization_ttl_days` and never read it. `auth_users.date_active` was only the bind time, so treating it as last activity on deploy would delete sessions that are still in use. A ping or RPC now stores `date_active` (at most once an hour) and sets `activity_trusted`. Expiry runs only after that tracked activity has been idle for the full TTL. Rows that have never recorded a ping stay. The caller that is pinging is refreshed before the check, so it is not deleted. `tools/expireauthorizations` uses the same rule. Its backup tables are created before the transaction, and `--rollback` restores the authorization row and the device token. It is not run from deploy.

| Piece | File |
| --- | --- |
| stable hash, temp-key adopt, TTL, push unbind | `app/service/authsession/internal/dao/auths.go`, `authorization.go`, `authorization_ttl.go` |
| hash SQL | `app/service/authsession/internal/dal/dao/mysql_dao/auth_users_dao.go` |
| auth state and bind | `app/service/authsession/internal/core/authsession.getAuthStateData_handler.go`, `authsession.bindTempAuthKey_handler.go` |
| 401 on ping and the next RPC | `app/interface/session/internal/sess/auth_state.go`, `session_service_messages.go`, `session_invoke.go` |
| inactivity rule | `pkg/authzttl` |
| trusted activity column | `teamgramd/deploy/sql/migrate-20261112-authorization-activity.sql` |
| operator script | `tools/expireauthorizations` |
| password-step recheck | `app/interface/session/internal/sess/main_auth_wrapper.go` |

## Verification quotas (A05 / A06)

Phone and email codes share `pkg/verify.Limiter`. The counter is one Redis `INCR` per target per Asia/Shanghai calendar day. The default cap is 10 (`TELLOMI_SMS_DAILY_LIMIT`, `TELLOMI_EMAIL_DAILY_LIMIT`). The 11th call returns `PHONE_NUMBER_FLOOD` before the SMS or email sender runs. A number outside `TELLOMI_SMS_COUNTRY_ALLOW` (default `86`) returns `PHONE_NUMBER_INVALID` before that sender. `TELLOMI_SMS_ALLOW_INTERNATIONAL=true` accepts other countries again. Resend, change-phone, confirm-phone, login email, and cloud-password email use this gate. The contract for clients is `docs/api/verification-limits.md`.

## Cloud password (SRP)

`account.getPassword` used to report that no password was set, and `account.updatePasswordSettings` did not store a verifier. `auth.checkPassword` did not check one. Login copied the user id into `SESSION_PASSWORD_NEEDED_<id>` so the session process could enter the password step; upstream iOS compares the error text exactly.

The verifier, salts, hint, recovery email, secure settings, and pending reset now live in MySQL (`user_passwords`, `user_password_srp`, `user_password_login`; `teamgramd/deploy/sql/migrate-20261101-cloud-password.sql`). SRP is `passwordKdfAlgoSHA256SHA256PBKDF2HMACSHA512iter100000SHA256ModPow` with the standard 2048-bit parameters. Each `srp_id` / `B` is deleted when it is used. Recovery mail uses the existing email sender (`TELLOMI_EMAIL_PROVIDER`).

`auth.signIn`, the email and future-auth-token paths, and `auth.exportLoginToken` return `SESSION_PASSWORD_NEEDED` with no suffix. The user id is stored on `user_password_login` before the auth key is bound. The session process still accepts the old suffixed form.

After `auth.logOut` on the same permanent key, that error moves the session from Logout, Unknown, or New to NeedPassword. Only an already Normal session ignores it, so a delayed error cannot undo `auth.checkPassword`. `auth.checkPassword` success moves every other state to Normal. `account.getPassword`, `auth.requestPasswordRecovery`, `auth.checkRecoveryPassword`, and `auth.recoverPassword` read `user_password_login` when the session user id is still 0, the same fallback `auth.checkPassword` already had. A reconnect that finds the key in `auth_users` before the password check stays on NeedPassword while that row exists.

iOS first login (phone, SMS code, then the cloud password) used to die on the password screen, while Add Account after an email login succeeded. The first connection's temp key is not bound yet, so the gateway forwarded those RPCs with perm id 0. Every such connection shared one session. `auth.signIn` still returned `SESSION_PASSWORD_NEEDED`, and `account.getPassword` answered with a password object that has no `current_algo`. iOS `verifyPassword` then fails and does not retry `SRP_ID_INVALID` when the temp key and the permanent key disagree. Add Account runs after `auth.bindTempAuthKey`, so that session already has the permanent key and SRP data. An unbound temp key gets a session under its own id, not perm id 0, so a ping sent before `auth.bindTempAuthKey` still receives `bad_server_salt`, `new_session_created` and `pong`. The bind message switches later traffic onto the permanent key. A permanent key whose stored perm column is 0 is treated as its own key. A proof is accepted when `srp_id` and M1 match, even if the two calls used different auth keys.

Disabling 2FA sends `new_algo = passwordKdfAlgoUnknown` and an empty `new_password_hash` (hint and email may be present and empty). That removes the verifier, recovery email, and secure settings after the current SRP check, and drops `user_password_login` rows. Each `account.getPassword` deletes this user's expired `user_password_srp` rows. A live `srp_id` stays valid until it expires or the password changes.

| Method | File |
| --- | --- |
| `account.getPassword` | `app/bff/twofa/internal/core/account.getPassword_handler.go` |
| `account.getPasswordSettings` | `app/bff/twofa/internal/core/account.getPasswordSettings_handler.go` |
| `account.updatePasswordSettings` | `app/bff/twofa/internal/core/account.updatePasswordSettings_handler.go` |
| `account.confirmPasswordEmail` | `app/bff/twofa/internal/core/account.confirmPasswordEmail_handler.go` |
| `account.resendPasswordEmail` | `app/bff/twofa/internal/core/account.resendPasswordEmail_handler.go` |
| `account.cancelPasswordEmail` | `app/bff/twofa/internal/core/account.cancelPasswordEmail_handler.go` |
| `account.declinePasswordReset` | `app/bff/twofa/internal/core/account.declinePasswordReset_handler.go` |
| `account.getTmpPassword` | `app/bff/twofa/internal/core/account.getTmpPassword_handler.go` |
| `auth.checkPassword` | `app/bff/authorization/internal/core/auth.checkPassword_handler.go` |
| `account.resetPassword` | `app/bff/authorization/internal/core/account.resetPassword_handler.go` |

Other touched upstream files:

- `app/bff/authorization/internal/core/auth.signIn_handler.go`, `auth.sendCode_handler.go`, `email_code.go` — exact `SESSION_PASSWORD_NEEDED` after the phone or email code is accepted.
- `app/bff/qrcode/internal/core/auth.acceptLoginToken_handler.go`, `auth.exportLoginToken_handler.go` — same for QR login.
- `app/bff/codelogin/methods.go` — `auth.requestPasswordRecovery` / `auth.recoverPassword` read and write the stored recovery email and verifier instead of a second copy.
- `app/interface/session/internal/sess/session_invoke.go` — exact error, user id from the auth-key binding.
- `app/service/authsession/internal/core/authsession.getAuthStateData_handler.go` — a pending password login stays `AuthStateNeedPassword` after a restart.
- `app/bff/bff/internal/server/server.go` — opens the password store from the existing MySQL DSN.

## P0 login, email, and phone-confirmation RPCs

The generated handlers below were enterprise stubs (`ERR_ENTERPRISE_IS_BLOCKED`). They now follow the current method pages on core.telegram.org. Delivery reuses the existing Aliyun SMS sender (one sign name, one login template) and the existing email sender (`TELLOMI_EMAIL_PROVIDER`, Resend in production). No new environment variables and no new SMS template.

| Method | File |
| --- | --- |
| `account.sendConfirmPhoneCode` | `app/bff/account/internal/core/account.sendConfirmPhoneCode_handler.go` |
| `account.confirmPhone` | `app/bff/account/internal/core/account.confirmPhone_handler.go` |
| `account.sendVerifyEmailCode` | `app/bff/authorization/internal/core/account.sendVerifyEmailCode_handler.go` |
| `account.verifyEmail` | `app/bff/authorization/internal/core/account.verifyEmail32DA4CF_handler.go` |
| `auth.requestPasswordRecovery` | `app/bff/authorization/internal/core/auth.requestPasswordRecovery_handler.go` |
| `auth.checkRecoveryPassword` | `app/bff/authorization/internal/core/auth.checkRecoveryPassword_handler.go` |
| `auth.recoverPassword` | `app/bff/authorization/internal/core/auth.recoverPassword_handler.go` |
| `auth.resetLoginEmail` | `app/bff/authorization/internal/core/auth.resetLoginEmail_handler.go` |
| `auth.reportMissingCode` | `app/bff/authorization/internal/core/auth.reportMissingCode_handler.go` |
| `auth.requestFirebaseSms` | `app/bff/authorization/internal/core/auth.requestFirebaseSms_handler.go` |
| `account.invalidateSignInCodes` | `app/bff/authorization/internal/core/account.invalidateSignInCodes_handler.go` |

State for those RPCs lives in `app/bff/codelogin` (redis when the BFF KV is configured, memory otherwise).

Other touched upstream files:

- `app/bff/authorization/internal/core/auth.signIn_handler.go` — a phone-code record whose type is `sentCodeTypeEmailCode` (set by `auth.resetLoginEmail` or email login setup) is checked against the stored code, same as email login.
- `app/bff/authorization/internal/core/auth.sendCode_handler.go` and `email_code.go` — remember the verified login email and index the login code so `account.invalidateSignInCodes` can drop it.
- `app/bff/account/internal/dao/dao.go` — `Cache()` exposes the existing account redis client.
- `proto/mtproto/rpc_error_codes.go` — `EMAIL_NOT_ALLOWED` and `EMAIL_INSTALL_MISSING`, both already documented on the method pages.

`account.verifyEmail#ecba39db` (the pre-purpose constructor) is unchanged and still returns the enterprise error. Layer 232 clients call `account.verifyEmail#32da4cf`.

Email-only accounts keep the login identity in `users.email` (`users.phone` NULL). `account.getPassword` used to leave `login_email_pattern` empty, so settings asked the user to bind an email they had already registered with. The pattern is now the masked `users.email`. It is a read of that column. Sign-up is still the only writer; this does not merge users or rewrite `user.id` or sessions.

`account.sendVerifyEmailCode` / `account.verifyEmail` with `emailVerifyPurposeLoginChange` or `emailVerifyPurposeLoginSetup` return `EMAIL_NOT_ALLOWED` when the address is not that `users.email`, or when it is already another account's `users.email`. The Redis key `tellomi:p0:loginemail:{userId}` is not updated in those cases. Confirming the same identity address is allowed. `auth.recoverPassword` may still store a different cloud-password recovery address in `user_passwords.email`; a refusal to copy that address onto the Redis login key does not abort recovery. No schema migration.

A Redis copy that already differs from `users.email` is left in place. It is not scanned and not rewritten. Before any later `auth.sendCode` stores the identity address again, readers that send mail do not use that copy: `auth.requestPasswordRecovery` on an email-only account with no cloud password returns `PASSWORD_RECOVERY_NA`, and `auth.resetLoginEmail` for a phone account skips a copy that is another account's `users.email`. The 180-second `tellomi:p0:pendingemail` fallback is skipped the same way. Email `auth.resetLoginEmail` sends to the address in the request. `help.getPromoData` drops `SETUP_LOGIN_EMAIL` and `SETUP_LOGIN_EMAIL_NOSKIP`. `login_email_pattern` is only the mask.

An empty DSN, a failed `sql.Open`, a failed ping, a missing `users` row, or a non-address in `users.email` is not a confirmed phone account. `account.getPassword` then returns `INTERNAL_SERVER_ERROR` instead of a password object with an empty `login_email_pattern`. Recovery without a cloud password does not mail Redis. A confirmed phone row (NULL email) still may. Cloud-password recovery keeps using `user_passwords.email`.

- `app/bff/codelogin/methods.go`, `service.go` — mail destinations ignore a divergent or cross-user Redis copy.
- `app/bff/helpdesk/suggestions.go` — those two suggestion names are not offered.

- `app/bff/twofa/password/service.go`, `identity.go`, `mysql.go` — `login_email_pattern` from `SELECT email FROM users`.
- `app/bff/codelogin/service.go`, `methods.go` — refuse a divergent or cross-user login email.
- `app/bff/authorization/internal/core/p0_codes.go` — wires those checks to the password store's read of `users.email`.

`auth.requestFirebaseSms` returns `boolFalse` for a well-formed request. Official Android treats that as "use `auth.resendCode` / `next_type`" (`PLAYINTEGRITY_REQUESTFIREBASESMS_FALSE`). Firebase is not used in mainland China. `PHONE_CODE_EMPTY` and `PHONE_NUMBER_INVALID` are still returned when the request itself is invalid.

`account.sendConfirmPhoneCode` returns `HASH_INVALID` unless a confirmation hash was issued. `account.deleteAccount` still deletes immediately, so production does not mint that hash. When a hash exists, the SMS uses the same Aliyun login template, the 10 second cap, and `SMS_CODE_CREATE_FAILED`.

## Trace export endpoint

`teamgramd/etc2/*.yaml` sets `Telemetry.Endpoint` to `${TELLOMI_OTEL_ENDPOINT}`. Service entrypoints load that yaml through `pkg/configload.MustLoad` (`conf.UseEnv`). When the variable is unset or empty, `Telemetry.Disabled` is set so go-zero does not install an exporter. Upstream files: each `internal/server/server.go` that called `conf.MustLoad`.

## Logging that `go vet` rejected

Handshake logs in `app/interface/gnetway` and `app/interface/httpserver` passed a `HandshakeStateCtx` to `%s`. The type has no `String` method. The format is `%v`.

`httpserver` session discovery called `logx.Error` with a format string. That is `logx.Errorf`.

`app/service/dfs/internal/ffmpegutil` logged `InitializeEmptyTranscoder` with `%v` and no error value. The error is the second argument. The dfs example programs under `internal/dao/examples` used `logx.Error` / `logx.Infof` with the wrong arity.

## Tests that need fixtures

JPEG tests skip when `app/service/dfs/internal/imaging/testdata` is absent. `pkg/goffmpeg/tests` skips when `/tmp/ffmpeg/3gp` or the `ffmpeg` binary is missing. `TestInputNotFound` still runs: a missing input is what it asserts. `ffmpegutil`'s gif test skips the same way.

## Group read receipts

`messages.readHistory` looks up `user_message_box_id` without a peer. Those ids are per user, so a basic-group read can hit an older private message that reused the number. The read then succeeds and the group's `read_outbox_max_id` stays 0. `messageForRead` keeps a row only when its peer is the chat being read, and otherwise maps the id through `dialog_message_id` from another participant.

## Dialog unread marks

`dialog.markDialogUnread` and `dialog.getDialogUnreadMarkList` were stubs (`METHOD_NOT_IMPL`). `messages.markDialogUnread` and `messages.getDialogUnreadMarks` already called them. The dialog rows now store the flag in `dialogs.unread_mark`.

## Session authorization settings

`account.changeAuthorizationSettings` was an enterprise stub. It now stores `encrypted_requests_disabled`, `call_requests_disabled`, and `unconfirmed` on `auth_users` (`teamgramd/deploy/sql/migrate-20261103-authorization-settings.sql`) and `account.getAuthorizations` reads them back. `hash` 0 is the current permanent key. A missing hash is `HASH_INVALID`. A new `auth_users` row is unconfirmed when the user already has another live session; the flag drops off the authorization list after 7 days (`authorization_autoconfirm_period` in `help.getAppConfig`). Login does not yet push `updateNewAuthorization`, and the flags do not filter incoming calls or secret chats.

| Method | File |
| --- | --- |
| `account.changeAuthorizationSettings` | `app/bff/authorization/internal/core/account.changeAuthorizationSettings_handler.go` |
| `account.getAuthorizations` | `app/service/authsession/internal/dao/authorization.go` |

## Datacenter addresses (`help.getConfig`)

`teamgramd/bin/config.json` ships `dc_options: []`, `dc_txt_domain_name: apv2.stel.com`, and `webfile_dc_id: 1`. `help.getConfig` clones that file once per request. When `TELLOMI_DC_OPTIONS` is a valid `dcId:ipv4:port` list, the clone's `dc_options` is replaced and `webfile_dc_id` is set to 1. Unset, empty, or invalid leaves both fields as loaded. An invalid spec is logged once and the process stays up. `TELLOMI_PUBLIC_DC_HOST`, when it is a canonical dotted IPv4, then replaces dotted IPv4 addresses in that list. An `ipv6` flag or a literal containing `:` (including `::ffff:x.x.x.x`) is not replaced. Entries whose port is not 10443 or 443 are dropped from the reply and logged on every call. A DC left with no remaining port falls back to that same DC on 10443 and 443. An empty input publishes DC 1 only. If `this_dc` or the file DC then has no 10443 or 443 address, or `TELLOMI_DC_OPTIONS` is invalid, the rewrite is not applied and the process keeps serving the list already loaded. The variable is not applied to gnetway listen addresses. `TELLOMI_DC_TXT_DOMAIN` set to `-` blanks the TXT domain; empty leaves `apv2.stel.com`.

The only upstream file is `app/bff/configuration/internal/core/help.getConfig_handler.go`, which calls `pkg/dcoptions.Apply` on the clone. Parsing and validation live in `pkg/dcoptions`.

## coturn entrypoint

`docker-compose.yaml` used to set coturn `entrypoint` to `/opt/coturn/entrypoint.sh`. That path exists only because the same file bind-mounts `teamgramd/deploy/coturn/entrypoint.sh`. A host override that replaces volumes (`volumes: !override`) removed the file and coturn crash-looped. Chengdu set `entrypoint: !reset null` so the image entrypoint ran. The compose entrypoint now runs the mounted generator when the file is there, and `docker-entrypoint.sh` with the image command otherwise. The image still reads `/etc/coturn/turnserver.conf` when the override mounts one. The generator script is Tellomi's; it is not an upstream file.

## Language list (`langpack.getLanguages`, `help.getConfig`)

Hindi, Thai, and Greek were listed with `official` set, but Telegram's export for them is still partial (Thai is an English copy, which is not shipped). The gaps are machine-translated so each pack has the same keys as English, marked `mt:true` for review. `TELLOMI_LANGPACK_HIDDEN` (unset means `hi,th,el`; empty lists every pack) keeps those codes out of `langpack.getLanguages` until that review. `langpack.getLanguage`, `langpack.getLangPack`, `langpack.getStrings`, and `langpack.getDifference` still serve them.

`help.getConfig` `suggested_lang_code` follows `initConnection.system_lang_code`, the same way official Telegram does. An empty device language uses `lang_code`. A hidden language is not suggested. `lang_pack_version` still follows the app language.

The only upstream file is `app/bff/configuration/internal/core/help.getConfig_handler.go` (the comment on the existing `ApplyConfigLanguage` call). The list and the suggestion live in `pkg/langpack`.

## Sticker wire format

`stickerSet` flags bit 4 is `thumbs`, `thumb_dc_id`, and `thumb_version` together. The catalog used to set the ints without a thumbs vector, so `messages.featuredStickers` did not round-trip (`DecodeVectorLong`). The catalog now sends `thumb_document_id` (flags bit 8) and leaves bit 4 unset. The generated codec in `proto/mtproto` is unchanged.

## Username policy (M5 / D-24 / D-56)

`TELLOMI_USERNAME_POLICY=1` runs `pkg/usernamepolicy` before the username table. Unset or any other value leaves these handlers as they were. A username hit returns `USERNAME_OCCUPIED`. A display-name hit returns `FIRSTNAME_INVALID`, or `LASTNAME_INVALID` when the field is the last name. The handler logs `rule_id`. The response does not carry the rule. The lexicon and the flag live in Tellomi files (`pkg/usernamepolicy`, `docker-compose.yaml`).

| Call | File | Hook |
| --- | --- | --- |
| `account.checkUsername` | `app/bff/usernames/internal/core/account.checkUsername_handler.go` | `usernamepolicy.RejectUser` after the format check |
| `account.updateUsername` | `app/bff/usernames/internal/core/account.updateUsername_handler.go` | `usernamepolicy.RejectUser` in `updateUsername` after the format check |
| `channels.checkUsername` | `app/service/biz/channel/service.go` `CheckUsername` | `usernamepolicy.RejectChannel` after the format check |
| `channels.updateUsername` | `app/service/biz/channel/service.go` `UpdateUsername` | `usernamepolicy.RejectChannel` after the format check, skipped when the name is cleared |
| `bots.checkUsername` | `app/bff/botadmin/bots.go` `CheckUsername` | `usernamepolicy.RejectBot` after the format check |
| `bots.createBot` | `app/bff/botadmin/bots.go` `CreateBot` | `usernamepolicy.RejectBot` after the format check |
| `bots.addUsername` | `app/bff/botadmin/more.go` `AddUsername` | `usernamepolicy.RejectBot` after the format check |
| `account.updateProfile` | `app/bff/userchannelprofiles/internal/core/account.updateProfile_handler.go` | `usernamepolicy.RejectDisplay` when first name, last name, or about changes. `FIRSTNAME_INVALID` or `LASTNAME_INVALID` |
| `channels.createChannel` | `app/service/biz/channel/service.go` `Create` | `usernamepolicy.RejectDisplay` on the title and the about text. `FIRSTNAME_INVALID` |
| `channels.editTitle` | `app/service/biz/channel/service.go` `EditTitle` | `usernamepolicy.RejectDisplay` when the title changes. `FIRSTNAME_INVALID` |
| `messages.editChatAbout` | `app/service/biz/channel/service.go` `EditAbout` | `usernamepolicy.RejectDisplay` when a channel or supergroup description changes. `FIRSTNAME_INVALID` |
| `messages.createChat` | `app/bff/chats/internal/core/messages.createChat_handler.go` and `app/service/biz/chat/internal/core/chat.createChat2_handler.go` | `usernamepolicy.RejectDisplay` on the basic-group title. `FIRSTNAME_INVALID` |
| `messages.editChatTitle` | `app/service/biz/chat/internal/core/chat.editChatTitle_handler.go` | `usernamepolicy.RejectDisplay` when the basic-group title changes. `FIRSTNAME_INVALID` |
| `messages.editChatAbout` | `app/service/biz/chat/internal/core/chat.editChatAbout_handler.go` | `usernamepolicy.RejectDisplay` when the basic-group description changes. `FIRSTNAME_INVALID` |
| `bots.createBot` | `app/bff/botadmin/bots.go` `CreateBot` | `usernamepolicy.RejectDisplay` on the bot name. `FIRSTNAME_INVALID` |
| `bots.setBotInfo` | `app/bff/botadmin/bots.go` `SetBotInfo` | `usernamepolicy.RejectDisplay` when the bot name, about, or description changes. `FIRSTNAME_INVALID` |

There is no separate bot primary-username rename RPC. `bots.checkUsername` is the probe, `bots.createBot` writes the first name, and `bots.addUsername` writes another name. All three call the same username hook. Display names, bios, and group titles use `RejectDisplay` and do not return `USERNAME_OCCUPIED`. The lexicon has no separate `bio` field, so a bio is checked as `display_name`. An unchanged bio or title is left alone. Chat messages are not checked. Sign-up is not checked.

`TELLOMI_CONTENT_SAFETY` adds Aliyun Green (`1` or `aliyun`, `TextModeration` / `nickname_detection`) or Bailian text moderation (`bailian`, `TextModerationPlus` / `nickname_detection_pro`) after the lexicon, for Han text only. Unset is off. A block uses the same `FIRSTNAME_INVALID` / `LASTNAME_INVALID` errors. A timeout, HTTP error, or missing key allows the name and logs the failure. The client does not see the label. The call lives in `pkg/contentsafety` and is reached from `RejectDisplay`.

## Discover credential (M3)

`TELLOMI_DISCOVER=1` makes `discover.getEngineCredential#a80a7f76` return a content-engine HS256 JWT, and `discover.getAudienceToken#dc5213c2` return an audience JWT (`typ` `tellomi-feed-audience`, at most 500 member pseudonyms, 10 minute `exp`). Any other value, including unset, answers `DISCOVER_DISABLED` for both. The credential schema is `docs/discover-engine-auth.md`. The audience client contract is `docs/api/discover-ai.md`. The codec and the handlers live in Tellomi files (`proto/mtproto/discover_codec.go`, `app/bff/discover`). Login and session routing are unchanged: both methods use the route that was already registered for `RPCDiscover`.

| Call | File | Hook |
| --- | --- | --- |
| session route | `teamgramd/etc/session.yaml`, `teamgramd/etc2/session.yaml`, `app/interface/session/etc/session.yaml` | `"/mtproto.RPCDiscover": "bff.bff"` |
| bff register | `app/bff/bff/internal/server/server.go` | `RegisterRPCDiscoverServer` |
| RPC table | `proto/mtproto/rpc_client_registers.go` | `TLDiscoverGetEngineCredential`, `TLDiscoverGetAudienceToken` |

## Discover relations (M3 / D-13 / D-16 / D-43)

`TELLOMI_DISCOVER_RELATIONS=1` serves `POST /internal/v1/relations/check` so content-engine can ask whether a viewer `sub` is in an author's contacts or close friends. The read is live (`user_contacts`, `is_deleted = 0`, `close_friend`). The contract is `docs/discover-relations.md`. `discover_subject` is `teamgramd/deploy/sql/migrate-20261104-discover-subject.sql`.

| Call | File | Hook |
| --- | --- | --- |
| start listener | `app/bff/bff/internal/server/server.go` | `discoverrel.Start` |

## Discover profile cards (M3 / D-17)

`TELLOMI_DISCOVER_PROFILES=1` serves `POST /internal/v1/profiles/resolve` so content-engine can read a viewer's copy of an author's name, username, bio, and profile photo. Photo and bio follow `user_privacies` (`key_type` 6 and 10) and the author's live `user_contacts` row. Unset does not listen. Login and `messages.sendMessage` are unchanged. The contract is `docs/discover-profiles.md`. No new table.

| Call | File | Hook |
| --- | --- | --- |
| start listener | `app/bff/bff/internal/server/server.go` | `discoverprofile.Start` |

## Xiaomi push (M8 / D-68)

`TELLOMI_XIAOMI_PUSH=1` sends token type 14 through MiPush (`pkg/push/xiaomi.go`). Unset leaves the token stored and unsent. `account.registerDevice` is unchanged: it already stores any positive `token_type`. Login and session routing are unchanged. The contract is `docs/push.md`.

| Call | File | Hook |
| --- | --- | --- |
| sync startup | `app/messenger/sync/internal/svc/service_context.go` | `push.LoadXiaomi` then `SetXiaomi` |
| deliver | `pkg/push/service.go` | `TokenXiaomi` |

## FCM and Huawei push (M8 / D-68)

`TELLOMI_FCM_PUSH=1` sends token type 2 through FCM HTTP v1 (`pkg/push/fcm.go`). `TELLOMI_HUAWEI_PUSH=1` sends token type 13 through Huawei Push Kit (`pkg/push/huawei.go`). Unset leaves each token stored and unsent. No new RPC and no new table. The FCM attempt does not share its deadline with APNs, Huawei, or Xiaomi. The contract is `docs/push.md`.

| Call | File | Hook |
| --- | --- | --- |
| sync startup | `app/messenger/sync/internal/svc/service_context.go` | `push.LoadFCM` then `SetFCM`; `push.LoadHuawei` then `SetHuawei` |
| deliver | `pkg/push/service.go` | `TokenFCM`, `TokenHuawei`, including `phoneCallRequested` |

## Discover push (M3 / D-18)

`TELLOMI_DISCOVER_PUSH=1` serves `POST /internal/v1/discover/notify` on the sync process. content-engine posts an interaction; teamgram resolves `sub` with `discover_subject` and sends it through `pkg/push` (APNs, FCM, Huawei, and Xiaomi when that sender is installed). Muting user 777005 (`@faxian`) with `account.updateNotifySettings` suppresses it. Unset does not listen and does not change chat push. The client contract is `docs/api/discover-ai.md`. `discover_push_event` is `teamgramd/deploy/sql/migrate-20261106-discover-push.sql`.

| Call | File | Hook |
| --- | --- | --- |
| start listener | `app/messenger/sync/internal/svc/service_context.go` | `discoverpush.Start` |

## @tellomi AI (M4, phase 1 server)

`TELLOMI_AI=1` plus `TELLOMI_AI_API_KEY` replies to private messages sent to user 777001 and can serve `POST /internal/v1/ai/complete` for a content-engine @mention. The model runs only for usernames in `TELLOMI_AI_ALLOW_USERNAMES` (default `erel,lfnuebfci,pikaqiu`; `*` allows everyone) and ids in `TELLOMI_AI_ALLOW_USER_IDS`. Anyone else, an empty or non-text message, a quota, a content-safety block, and a model failure each get a fixed sentence and do not ask the model to write that sentence. With the flag off, the same chats get the not-open sentence and the engine endpoint stays down. `messages.sendMessage` returns the user's update before the reply is generated. Login and session routing are unchanged. The contract is `docs/api/tellomi-ai.md`. `tellomi_ai_reply` is `teamgramd/deploy/sql/migrate-20261106-tellomi-ai.sql`.

| Call | File | Hook |
| --- | --- | --- |
| `messages.sendMessage` | `app/bff/messages/internal/core/messages.sendMessage_handler.go` | `maybeTellomi`, only after a successful send |
| `messages.sendMedia` / `messages.sendMultiMedia` | `app/bff/messages/internal/core/messages.sendMedia_handler.go`, `messages.sendMultiMedia_handler.go` | `noteTellomi`; a caption is text, media without text is the text-only sentence |
| bff start | `app/bff/bff/internal/server/server.go` | `bots.StartTellomi` |
| app config | `app/bff/configuration/internal/appconfig/app_config.go` | `tellomi_ai_enabled` |

## Launch notice

`TELLOMI_ANNOUNCE=1` plus `TELLOMI_ANNOUNCE_KEY` serves `POST /internal/v1/announce` on bff. Each page writes up to 50 private messages from user 777000 (`@admin`) and one channel post in `@news`. Unset does not listen. Login and `messages.sendMessage` are unchanged. The contract is `docs/api/announce.md`. `tellomi_announcement` is `teamgramd/deploy/sql/migrate-20261108-announce.sql`.

| Call | File | Hook |
| --- | --- | --- |
| bff start | `app/bff/bff/internal/server/server.go` | `bots.StartAnnounce` |
| app config | `app/bff/configuration/internal/appconfig/app_config.go` | `tellomi_announce_enabled` |

## User reports and the @admin receipt

`account.reportPeer`, `account.reportProfilePhoto`, `messages.report` (both constructors), `messages.reportSpam`, `messages.reportReaction`, `messages.reportEncryptedSpam`, `channels.reportSpam`, `stories.report`, and `ephemeral.reportMessage` insert one `reports` row and send a private message from user 777000 (`@admin`) to the reporter. The receipt does not depend on `TELLOMI_REPORT_REVIEW` or `TELLOMI_REPORT_HTTP`. An empty `TELLOMI_REPORT_REVIEW` classifies the row when `TELLOMI_AI=1` and `TELLOMI_AI_API_KEY` are set. `TELLOMI_REPORT_REVIEW=0` leaves it for a person. `TELLOMI_REPORT_HTTP=1` serves `POST /internal/v1/reports` for Discover and `POST /internal/v1/reports/subjects` so content-engine can resolve a `sub` through `discover_subject`. Login is unchanged. The contract is `docs/api/reports.md`. `reports` is `teamgramd/deploy/sql/migrate-20261105-reports.sql` and is also created when bff opens MySQL.

| Call | File | Hook |
| --- | --- | --- |
| bff start | `app/bff/bff/internal/server/server.go` | `bots.StartReports` |
| peer / spam | `app/bff/channels/internal/server/grpc/service/handlers_p1.go` | `report.Shared().Submit` |
| encrypted spam | `app/bff/channels/internal/server/grpc/service/handlers_report.go` | same |
| ephemeral | `app/bff/ephemeral/internal/box/box.go` | same |

## Discover account deletion (M3)

`TELLOMI_DISCOVER_DELETE=1` calls content-engine `DELETE /internal/users/{sub}` after `UserDeleteUser` has succeeded. The call is queued in memory, retried, and cannot fail or delay `account.deleteAccount`. See `docs/discover-account-deletion.md`.

| Call | File | Hook |
| --- | --- | --- |
| `account.deleteAccount` | `app/bff/account/internal/core/account.deleteAccount_handler.go` | `discoverdelete.Enqueue` |

## Discover link cards

`TELLOMI_LINK_PREVIEW=1` serves `POST /internal/v1/link-preview` on bff so content-engine can build a Discover card. Unset does not listen. The fetch is the same SSRF-protected Open Graph read that chat used before external fetch was turned off: private, loopback, and link-local addresses are refused before the request and again after DNS and redirects. Bracketed IPv6 and `localhost` are refused before the request. A block after DNS or a redirect is HTTP 400. A relative image URL is resolved against the final page URL. The JSON card is title, description, image URL, and site name. It does not use the public tell.cc resolver on port 8801. The contract is `docs/api/link-preview-card.md`.

Chat cards are separate. Internal `tell.cc` / `t.me` links are built from local data. External pages are not fetched unless `TELLOMI_SERVER_LINK_FETCH=1`. A sender that fetched Open Graph on-device submits it as `inputMediaWebPage` flags.3–6. The contract is `docs/api/client-link-preview.md`.

| Call | File | Hook |
| --- | --- | --- |
| `messages.getWebPage` | `app/bff/messages/webpage` | `webpage.Get` |
| `messages.sendMessage` | `app/bff/messages/internal/core/messages.sendMessage_handler.go` | `webpage.PreviewMedia` |
| `messages.sendMedia` | `app/bff/messages/internal/core/core_messages_helper.go` | `webpage.Media` |
| bff start | `app/bff/bff/internal/server/server.go` | `chatbox.StartLinkPreview` |

## Chat primary and standby (M8 / D-70)

Design only. [docs/design/chat-standby.md](docs/design/chat-standby.md) describes a manual promotion of a MySQL replica, how `seqfloor` keeps Redis idgen and pts from moving backwards, and why the spare IP stays out of `TELLOMI_DC_OPTIONS` until the spare is listening. No code, no flag, and no migration.

## Translation and rich compose

`messages.translateText` and `messages.translateRichMessage` call only `TELLOMI_TRANSLATE_URL` (see `docs/api/translate.md`). They do not call Bailian. `TELLOMI_TRANSLATE=1` and `TELLOMI_AI_API_KEY` do not enable them. `messages.composeMessageWithAI` without `translate_to_lang` still calls Bailian when `TELLOMI_AI_COMPOSE=1` and `TELLOMI_AI_API_KEY` are both set. `messages.composeRichMessageWithAI` calls the same client when `TELLOMI_AI_COMPOSE=1` and the key are set. That second flag is what `help.getAppConfig` publishes as `tellomi_ai_compose_enabled`. The model is `qwen-plus`, then `deepseek-v3` on the same Bailian API. A content-safety block does not switch models. When `TELLOMI_CONTENT_SAFETY` is on, the output is checked again and a block is a failure. Unset flags, a missing key, and a provider failure return `500 TRANSLATE_REQ_FAILED` or `500 AICOMPOSE_TIMEOUT`. The previous English/Chinese word list is gone. Rich blocks stay in place; message entities keep their UTF-16 spans. `account.confirmBotConnection` is unchanged. Login and session routing are unchanged.

| Call | File | Hook |
| --- | --- | --- |
| `messages.translateText` | `app/bff/channels/internal/server/grpc/service/handlers_translate.go` | `chatbox.TranslateText` |
| `messages.translateRichMessage` | `app/bff/messages/internal/core/messages.translateRichMessage_handler.go` | `richmsg.Translate` |
| `messages.composeRichMessageWithAI` | `app/bff/messages/internal/core/messages.composeRichMessageWithAI_handler.go` | `richmsg.Compose` |
| app config | `app/bff/configuration/internal/appconfig/app_config.go` | `tellomi_translate_enabled`, `tellomi_ai_compose_enabled` |

## Help center link (M1 / D-40)

`TELLOMI_HELP_CENTER=1` sets `help.getAppConfig` `faq_link` to `https://tellomi.cn/help`. Unset leaves `https://tellomi.cn/faq`. The page is not served here. The hash of the app config includes the URL, so a client that caches `help.getAppConfig` refetches after the flag changes. Login is unchanged.

| Call | File | Hook |
| --- | --- | --- |
| `help.getAppConfig` | `app/bff/configuration/internal/appconfig/app_config.go` | `brand.HelpURL` |

## About page and recommendation default (D-50 / D-58 / D-59)

`TELLOMI_ABOUT_DISCLOSURE=1` plus `TELLOMI_ALGORITHM_FILING_NUMBER` sets `help.getAppConfig` `tellomi_personalized_recommend_default` to true and publishes `tellomi_algorithm_filing_number`. The same flag publishes `tellomi_ai_model_name` and `tellomi_ai_registration_number` when `TELLOMI_AI_PUBLIC_MODEL` and `TELLOMI_AI_REGISTRATION_NUMBER` are set. Unset leaves the bool false and omits the strings. The bool is the switch default, not a stored per-user choice. The hash includes the flag and the published strings. Login is unchanged.

| Call | File | Hook |
| --- | --- | --- |
| `help.getAppConfig` | `app/bff/configuration/internal/appconfig/app_config.go` | `pkg/about` |

## Login notice and session reset (N05 / N06)

`auth.signIn`, `auth.signUp`, `auth.checkPassword`, `auth.recoverPassword`, a future-auth-token `auth.sendCode` that finishes the login, and `auth.acceptLoginToken` when the account has no cloud password write one private message from user 777000. The text is Simplified Chinese. It names the new session's `initConnection` device model and the Beijing minute (UTC+8, no DST). It does not include the verification code, a token, or an auth key. A rejected code, `SESSION_PASSWORD_NEEDED`, `authorizationSignUpRequired`, a bad or replayed QR token, and a QR scan that still needs `auth.checkPassword` do not write it. The password scan writes it from `auth.checkPassword` after the proof matches, for that new key. The same user and auth key inside two minutes claims the message once. The same device model is what `auth.signIn` puts in the older service notification.

`account.resetAuthorization` and `auth.resetAuthorizations` return `406 FRESH_RESET_AUTHORISATION_FORBIDDEN` when the actor's own `auth_users.date_created` is under 24 hours. Nothing is deleted in that case. The target's age does not decide it. At 24 hours the other row is soft-deleted, and the old socket rechecks the binding on the next protected RPC, `ping`, and `ping_delay_disconnect`. A missed online update still fails the next RPC. Reconnect loads the deleted binding as logged out (`401 AUTH_KEY_UNREGISTERED`). Reset used to leave `date_created = 0` on that key; binding it again now starts the 24h gate at the new bind (`dao.bindDateCreated`). A live row keeps its original time.

`TELLOMI_MESSAGE_EXPIRY` and `TELLOMI_MESSAGE_EXPIRY_SWEEP` stay unset. This does not enable expiry cleanup.

| Piece | File |
| --- | --- |
| Notice text and device model | `app/bff/authorization/loginnotice`, `app/bff/authorization/internal/core/core.go` |
| QR login without a cloud password | `app/bff/qrcode/internal/core/auth.acceptLoginToken_handler.go` |
| Service-notification device | `app/bff/authorization/internal/core/auth.signIn_handler.go` |
| 24h gate and cleared `date_created` | `app/service/authsession/internal/dao/authorization.go`, `app/service/authsession/internal/dal/dao/mysql_dao/auth_users_dao.go` |
| Next RPC / ping | `app/interface/session/internal/sess/session_invoke.go`, `session_service_messages.go` |

## Official usernames (M5 P0 / D-27 / D-55 / D-60)

`account.checkUsername` reads the `username` table. The old `z_init.sql` wrote `tellomi` only on `users` (id 777000, `user_type` 4) and left `username` empty, so the name could still be taken. The rows that occupy the name live in Tellomi migrations, not in a second copy of the schema:

- `teamgramd/deploy/sql/migrate-20261104-tellomi-username.sql` inserts `@tellomi` (`peer_type` 2 = `PEER_USER`, `peer_id` 777000, `deleted` 0). `username.id` is `AUTO_INCREMENT`. Collation `utf8mb4_unicode_ci` makes `Tellomi` and `TELLOMI` the same key.
- `teamgramd/deploy/sql/migrate-20261105-official-accounts.sql` moves `@tellomi` to user 777001 and reserves the other appendix D names on that peer, on user 777000 (`@admin`), or on a channel (`peer_type` 4). One peer can own many `username` rows. The first name in each appendix D cell is `editable = 1`. The other names on that same peer stay `active = 1` and `editable = 0`, so `user.resolveUsername` opens them. Bot names are empty placeholders (`peer_type` 0, `peer_id` 0) because `bots.createBot` does not persist a bot this SQL can operate. `user.resolveUsername` returns `USERNAME_NOT_OCCUPIED` for peer type 0.
- `teamgramd/deploy/sql/migrate-20261107-tellomi-ai-name.sql` sets 777001's display name to Tellomi AI. 777000 stays the login-code account.
- `teamgramd/deploy/sql/migrate-20261109-appendix-c-placeholders.sql` inserts appendix C / D-56 names that appendix D does not assign, when they are valid public usernames, as the same empty placeholder. `qq`, `jd`, `x`, and `xai` are shorter than 4 characters and are not rows. Underscore affixes stay behind `TELLOMI_USERNAME_POLICY`. The runtime lexicon is now `policy-2026110602` (1024 rules). `policy-2026092301` remains only as the legacy corpus baseline. `allowlist.toml` is not in this repository.

`migrate.sh` records `z_init.sql` once. A database that already applied the old seed would never see a new `INSERT` there, so the occupation is a `migrate-*.sql` file. The one upstream edit is the seed's `users.username` for 777000: `tellomi` became `admin`, which is the appendix D name for that service account. The `INSERT IGNORE` still does not update an existing row. The official-accounts migration renames `tellomi` / `telegram` / `teamgram` / empty to `admin` on id 777000 only. Login-code sending is unchanged. Report handling is unchanged.

`0_migrate_helpers.sql` gained `teamgram_fail_if_set`. A name that already belongs to someone else stops the script (`D-26`). `SIGNAL` cannot run from a prepared statement (MySQL 1295), so the migrations `CALL` the procedure. How to apply this on a database that is already running, including the backup, is [docs/official-usernames.md](docs/official-usernames.md).

## Account enforcement

`pkg/enforcement` records platform sanctions on a user id. `Migrate` creates tables in the database named by `TELLOMI_ENFORCEMENT_MYSQL`; that name cannot be `teamgram`, and `OpenMySQL` does not run DDL. `TELLOMI_ENFORCEMENT_STORE=mysql` is the production switch that opens `TELLOMI_ENFORCEMENT_MYSQL`. `memory` and an empty value do not install the gate. Tables are created only by `/app/bin/enforcement-migrate`, not at startup. A substantiated user report is confirmed by a username in `TELLOMI_ENFORCEMENT_ADMINS` before a permanent sanction is approved. That same allowlist lifts one active sanction in the 777001 chat: the command is `解封案件` plus a sanction or review id, or `解封用户` plus a numeric user id, and the lift happens only after a later reply that is exactly `是`. The command does not listen on a port. The initiator still cannot lift a sanction they created, and `Revoke` still refuses the approver. Content decisions are `POST /internal/v1/enforcement/content/review` on `TELLOMI_ENFORCEMENT_ADDR` (docker network, not a published port). `TELLOMI_ENFORCEMENT_CREATE=1` is required before a new case can be written. Existing cases stay in force when creation is turned off. Rolling back the binary does not drop sanction, audit, or outbox rows. Notices are private messages from user 777000, written to an outbox in the same transaction as the case and delivered afterwards at least once. Appeals use `Judge`. When `TELLOMI_AI=1` and `TELLOMI_AI_API_KEY` are set, that judge is `pkg/tellomiai.Client`. The 7696-term list is not an input. Login, signup, future-auth bind, password recovery, QR accept, send/edit/forward, profile edit, phone change, discover credential, audience token, and the session invoke path call the same gate. A permanent login stashes a one-time disposition grant on the verified phone-code hash; `DispositionHandler` then reads 777000 notices and files that user's appeal. Credential recheck verifies the discover JWT and ignores a caller-supplied user id. Detail is [docs/account-enforcement.md](docs/account-enforcement.md) and [docs/api/enforcement-contract.md](docs/api/enforcement-contract.md).
