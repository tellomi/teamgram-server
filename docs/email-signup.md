# Email sign-up and login (design only)

This is a design note. The server does **not** implement email sign-up, login email, or the `account.sendVerifyEmailCode` / `account.verifyEmail` handlers. Those RPCs still return the existing not-implemented error.

`TELLOMI_EMAIL_PROVIDER` unset means email is off. The process does not need DirectMail settings to start. A `log` sender and an Aliyun DirectMail `SingleSendMail` client exist behind `verify.CodeSender` (`NewEmailSender`) for a later change; authorization does not call them. See [sms-email.md](sms-email.md).

Phone login is unchanged: SMS via Aliyun, or the fixed development code.

## What a later server change would add

Official clients already speak these layer-232 methods. The server would implement them without a new TL schema:

- `account.sendVerifyEmailCode` with `emailVerifyPurposeLoginSetup`, `emailVerifyPurposeLoginChange`, and `emailVerifyPurposePassport`.
- `account.verifyEmail` returning `account.emailVerified` or `account.emailVerifiedLogin`.
- `auth.sendCode` returning `auth.sentCodeTypeSetUpEmailRequired` when a new account must add an email, and `auth.sentCodeTypeEmailCode` when the account already has a login email.
- `auth.signIn` accepting `emailVerificationCode` instead of `phone_code` for that login.
- `auth.resetLoginEmail` falling back to SMS for that attempt.

DirectMail would be `SingleSendMail` on `https://dm.aliyuncs.com/`, region `cn-hangzhou`, from `noreply@mail.tellomi.cn` (`FromAlias` Tellomi), reusing `ALIBABA_CLOUD_ACCESS_KEY_ID` and `ALIBABA_CLOUD_ACCESS_KEY_SECRET`. Until the product is activated in Aliyun, the provider stays unset.

### Email-only accounts (no phone)

Not built. A follow-up would:

- Allow `users.phone` to be NULL (MySQL unique indexes allow many NULLs) and add `users.email varchar(255) NULL` with a unique index.
- Ship that as a one-shot migration, not as part of this SMS change.
- Teach the user service to insert NULL phone and the email, and to look up a user by email.
- Keep phone accounts unique on `users.phone`.

There is no stock MTProto method that creates an account with only an email. Patched clients would use one of these conventions, both of which need server support that is not here yet:

1. `auth.sendCode` / `auth.signIn` / `auth.signUp` with the email in `phone_number`.
2. `account.sendVerifyEmailCode` with `emailVerifyPurposeLoginSetup` whose `phone_number` is the email, then `account.verifyEmail`, then `auth.signIn` using `phone_code_hash` from `account.emailVerifiedLogin`.

Apple and Google `EmailVerification` tokens would be rejected.

## Client work (separate repos)

Unmodified Telegram iOS and DrKLO Android can only register and log in with a phone number. They already handle `sentCodeTypeEmailCode` and `sentCodeTypeSetUpEmailRequired` after `auth.sendCode`, and they submit `emailVerificationCode` on `auth.signIn`. That path starts working only after the server sends those types. No client change is required for phone login, with or without a future login email.

Email-only sign-up needs client changes:

**Telegram-iOS**

- The authorization sequence always collects a phone number before `auth.sendCode`. Add an email entry path that either calls `auth.sendCode` with the email in `phone_number`, or calls `account.sendVerifyEmailCode` (`emailVerifyPurposeLoginSetup`) and then `auth.signIn` / `auth.signUp` with that email.
- `auth.signUp` must send the email where it currently sends the phone, and must not require a SIM number.
- Code entry can stay on the existing email-code screen once the server returns `auth.sentCodeTypeEmailCode`.

**DrKLO Android** (`LoginActivity`)

- Phone entry is mandatory before `auth.sendCode`. Add the same email start path.
- Reuse the existing setup-email and email-code screens for `sentCodeTypeSetUpEmailRequired` and `sentCodeTypeEmailCode`.
- Pass the email through `auth.signUp` instead of a phone number, and do not block on a missing device phone number.

Those client edits are out of scope for this server change.
