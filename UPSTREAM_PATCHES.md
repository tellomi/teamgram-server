# Upstream patches

Tellomi changes on top of teamgram. Keep these small and listed here.

## Cloud password (SRP)

`account.getPassword` used to report that no password was set, and `account.updatePasswordSettings` did not store a verifier. `auth.checkPassword` did not check one. Login copied the user id into `SESSION_PASSWORD_NEEDED_<id>` so the session process could enter the password step; upstream iOS compares the error text exactly.

The verifier, salts, hint, recovery email, secure settings, and pending reset now live in MySQL (`user_passwords`, `user_password_srp`, `user_password_login`; `teamgramd/deploy/sql/migrate-20261101-cloud-password.sql`). SRP is `passwordKdfAlgoSHA256SHA256PBKDF2HMACSHA512iter100000SHA256ModPow` with the standard 2048-bit parameters. Each `srp_id` / `B` is deleted when it is used. Recovery mail uses the existing email sender (`TELLOMI_EMAIL_PROVIDER`).

`auth.signIn`, the email and future-auth-token paths, and `auth.exportLoginToken` return `SESSION_PASSWORD_NEEDED` with no suffix. The user id is stored on `user_password_login` before the auth key is bound. The session process still accepts the old suffixed form.

After `auth.logOut` on the same permanent key, that error moves the session from Logout, Unknown, or New to NeedPassword. Only an already Normal session ignores it, so a delayed error cannot undo `auth.checkPassword`. `auth.checkPassword` success moves every other state to Normal. `account.getPassword`, `auth.requestPasswordRecovery`, `auth.checkRecoveryPassword`, and `auth.recoverPassword` read `user_password_login` when the session user id is still 0, the same fallback `auth.checkPassword` already had. A reconnect that finds the key in `auth_users` before the password check stays on NeedPassword while that row exists.

iOS first login (phone, SMS code, then the cloud password) used to die on the password screen, while Add Account after an email login succeeded. The first connection's temp key is not bound yet, so the gateway forwarded those RPCs with perm id 0. Every such connection shared one session. `auth.signIn` still returned `SESSION_PASSWORD_NEEDED`, and `account.getPassword` answered with a password object that has no `current_algo`. iOS `verifyPassword` then fails and does not retry `SRP_ID_INVALID` when the temp key and the permanent key disagree. Add Account runs after `auth.bindTempAuthKey`, so that session already has the permanent key and SRP data. Unbound traffic is no longer given a session. A permanent key whose stored perm column is 0 is treated as its own key. A proof is accepted when `srp_id` and M1 match, even if the two calls used different auth keys.

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

## Sticker wire format

`stickerSet` flags bit 4 is `thumbs`, `thumb_dc_id`, and `thumb_version` together. The catalog used to set the ints without a thumbs vector, so `messages.featuredStickers` did not round-trip (`DecodeVectorLong`). The catalog now sends `thumb_document_id` (flags bit 8) and leaves bit 4 unset. The generated codec in `proto/mtproto` is unchanged.
