# Upstream patches

Tellomi changes on top of teamgram. Keep these small and listed here.

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
